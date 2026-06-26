<script>
  // Whole-cluster by-ear calibration (Nodes page). Starting it: stops playback,
  // joins EVERY playback speaker into THIS node's group (they follow self), and
  // plays a synchronized alignment signal to all of them. You pick which speakers
  // to hear with the mute toggles (NOT by regrouping — everyone stays joined), and
  // null each pair's flam with one compact list of centered ± delay sliders.
  // "Re-center" shifts every delay by a common amount (acoustically neutral — it
  // only changes absolute latency, not relative alignment) so the set sits around 0.
  import {
    assignToGroup,
    nodeSetVolume,
    nodeSetOutputDelay,
    calibrateStart,
    calibrateStop,
  } from "../lib/api.js";
  import { nameOf } from "../lib/derive.js";
  import CalibDelaySlider from "./CalibDelaySlider.svelte";
  import CalibVolSlider from "./CalibVolSlider.svelte";

  let { snapshot, self } = $props();

  const CAL_LEVEL = 0.6; // volume each speaker is set to while audible

  let active = $state(false);
  let mode = $state("click");
  let entryVol = new Map(); // pre-calibration volumes, restored on stop

  // Every playback-capable, alive speaker — the calibration roster.
  let speakers = $derived(
    (snapshot.nodes || [])
      .filter((n) => n && n.alive && n.capabilities && n.capabilities.playback)
      .sort((a, b) => (a.name || "").localeCompare(b.name || "")),
  );

  async function start() {
    entryVol = new Map(speakers.map((s) => [s.id, s.volume ?? 0]));
    try {
      // Join every other speaker into this node's group (current node = master).
      for (const s of speakers) {
        if (s.id !== self.id) await assignToGroup(s, self.id);
      }
      // Start all audible; you then mute to isolate the pair you're comparing.
      for (const s of speakers) await nodeSetVolume(s, CAL_LEVEL);
      await calibrateStart(self.id, { mode });
      active = true;
    } catch {
      /* toast shown by api */
    }
  }

  async function stop() {
    try {
      await calibrateStop(self.id);
    } finally {
      for (const s of speakers) {
        const v = entryVol.get(s.id);
        if (v !== undefined) nodeSetVolume(s, v).catch(() => {});
      }
      active = false;
    }
  }

  function setMode(m) {
    mode = m;
    if (active) calibrateStart(self.id, { mode: m }).catch(() => {});
  }

  function toggleAudible(node) {
    const audible = (node.volume ?? 0) > 0;
    nodeSetVolume(node, audible ? 0 : CAL_LEVEL).catch(() => {});
  }

  // Shift every delay by a common offset so the set centers on 0 (min→0 keeps them
  // all ≥0; avg→0 keeps them smallest). A uniform shift is acoustically neutral.
  function recenter(kind) {
    const ds = speakers.map((s) => s.outputDelayMs ?? 0);
    if (ds.length === 0) return;
    const offset =
      kind === "min"
        ? Math.min(...ds)
        : Math.round(ds.reduce((a, b) => a + b, 0) / ds.length);
    if (offset === 0) return;
    for (const s of speakers) {
      nodeSetOutputDelay(s, (s.outputDelayMs ?? 0) - offset).catch(() => {});
    }
  }
</script>

<section class="card calibrate">
  <header class="cal-head">
    <div>
      <h3>Calibrate speaker alignment</h3>
      <p class="cal-sub">
        Aligns every speaker against <strong>{self.name || "this node"}</strong>.
      </p>
    </div>
    {#if active}
      <button type="button" class="cal-btn stop" onclick={stop}>■ Stop</button>
    {:else}
      <button
        type="button"
        class="cal-btn start"
        onclick={start}
        disabled={speakers.length === 0}>▶ Start calibration</button
      >
    {/if}
  </header>

  {#if active}
    <p class="cal-hint">
      All speakers are joined here and playing the signal. <strong>Mute</strong> all
      but the two you're comparing, then slide one until the doubled click collapses
      to a single tick (switch to <strong>Noise</strong> for the final null). Delays
      save as you go; volumes restore when you stop.
    </p>

    <div class="cal-bar">
      <div class="cal-mode" role="group" aria-label="signal type">
        <button type="button" class:on={mode === "click"} onclick={() => setMode("click")}
          >Click</button
        >
        <button type="button" class:on={mode === "noise"} onclick={() => setMode("noise")}
          >Noise</button
        >
      </div>
      <div class="cal-recenter">
        <span class="lbl">Re-center:</span>
        <button type="button" onclick={() => recenter("min")} title="shift all so the lowest delay is 0"
          >min → 0</button
        >
        <button type="button" onclick={() => recenter("avg")} title="shift all so the average delay is 0"
          >avg → 0</button
        >
      </div>
    </div>

    <ul class="cal-list">
      {#each speakers as s (s.id)}
        {@const audible = (s.volume ?? 0) > 0}
        <li class="cal-row" class:muted={!audible}>
          <button
            type="button"
            class="cal-mute"
            class:on={audible}
            onclick={() => toggleAudible(s)}
            aria-pressed={audible}
            title={audible ? "mute" : "unmute"}>{audible ? "🔊" : "🔇"}</button
          >
          <span class="cal-name" title={nameOf(snapshot, s.id)}>{nameOf(snapshot, s.id)}</span>
          <CalibVolSlider node={s} />
          <CalibDelaySlider node={s} label={nameOf(snapshot, s.id)} />
        </li>
      {/each}
    </ul>
  {:else}
    <p class="cal-hint">
      Starting will <strong>stop playback</strong>, join all {speakers.length} speakers
      into this node's group, and play a test signal to all of them at once. Your
      groups and volumes are restored when you stop.
    </p>
  {/if}
</section>

<style>
  .calibrate {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin-bottom: 12px;
  }
  .cal-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
  }
  .cal-head h3 {
    margin: 0;
  }
  .cal-sub {
    margin: 2px 0 0;
    color: var(--muted);
    font-size: 12px;
  }
  .cal-hint {
    margin: 0;
    color: var(--muted);
    font-size: 12px;
    line-height: 1.5;
  }
  .cal-btn {
    font: inherit;
    cursor: pointer;
    color: inherit;
    background: transparent;
    border: 1px solid rgba(255, 255, 255, 0.18);
    border-radius: 6px;
    padding: 5px 14px;
    white-space: nowrap;
  }
  .cal-btn.start {
    border-color: color-mix(in srgb, var(--accent) 60%, transparent);
  }
  .cal-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .cal-bar {
    display: flex;
    align-items: center;
    gap: 16px;
    flex-wrap: wrap;
  }
  .cal-mode {
    display: inline-flex;
    border: 1px solid rgba(255, 255, 255, 0.15);
    border-radius: 6px;
    overflow: hidden;
  }
  .cal-mode button {
    font: inherit;
    cursor: pointer;
    color: var(--muted);
    background: transparent;
    border: 0;
    padding: 4px 12px;
  }
  .cal-mode button.on {
    color: inherit;
    background: rgba(255, 255, 255, 0.1);
  }
  .cal-recenter {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }
  .cal-recenter .lbl {
    color: var(--muted);
  }
  .cal-recenter button {
    font: inherit;
    font-size: 12px;
    cursor: pointer;
    color: inherit;
    background: transparent;
    border: 1px solid rgba(255, 255, 255, 0.15);
    border-radius: 4px;
    padding: 2px 8px;
  }
  .cal-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .cal-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .cal-row.muted {
    opacity: 0.5;
  }
  .cal-mute {
    font: inherit;
    cursor: pointer;
    background: transparent;
    border: 1px solid rgba(255, 255, 255, 0.15);
    border-radius: 4px;
    padding: 1px 6px;
    line-height: 1.2;
  }
  .cal-name {
    min-width: 8em;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
