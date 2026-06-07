<script>
  // One node in the Nodes table (J arch §4): editable name, id, addrs, caps,
  // liveness, volume, output-delay calibration.
  import { shortId, cidrList, relTime, ports } from "../lib/fmt.js";
  import {
    renameNode,
    setVolume,
    setOutputDelay,
    setOutputDevice,
    setDisabled,
    testTone,
  } from "../lib/api.js";
  import EditableText from "./EditableText.svelte";
  import VolumeSlider from "./VolumeSlider.svelte";

  let { node, self } = $props();

  let isSelf = $derived(node.id === self.id);
  let caps = $derived(node.capabilities || {});
  let portList = $derived(ports(node));

  // local draft for the output-delay input; reverts to node.outputDelayMs on
  // each new snapshot, committed on blur/Enter only (D36).
  let delayDraft = $state(0);
  $effect(() => {
    delayDraft = node.outputDelayMs ?? 0;
  });

  function commitDelay() {
    let ms = Math.round(Number(delayDraft));
    if (Number.isNaN(ms)) {
      delayDraft = node.outputDelayMs ?? 0;
      return;
    }
    if (ms > 500) ms = 500;
    if (ms < -500) ms = -500;
    if (ms === (node.outputDelayMs ?? 0)) return;
    setOutputDelay(node.id, ms).catch(() => {});
  }

  function onkey(e) {
    if (e.key === "Enter") {
      e.preventDefault();
      e.target.blur();
    }
  }

  // output-device selection (D37). List comes from the node's enumeration; the
  // <select> is hidden when empty (no ALSA on that host).
  let outputDevices = $derived(node.outputDevices ?? []);
  let outputDevice = $derived(node.outputDevice ?? "default");

  function onDeviceChange(e) {
    const dev = e.target.value;
    if (dev === outputDevice) return;
    setOutputDevice(node.id, dev).catch(() => {});
  }

  // Tri-state feature chips (D40): playback/opus/input are operator-toggleable.
  // `capabilities` is EFFECTIVE (probed minus disabled); `disabled` lists what
  // the operator turned off. Three UNMISTAKABLE states per feature:
  //   - "on"          → available + enabled (effective caps has F, not disabled):
  //                     green "●", clickable, tooltip "click to disable".
  //   - "off"         → available but disabled (F in node.disabled): amber outline
  //                     "○", clickable, tooltip "click to enable".
  //   - "unavailable" → not probed on this host (neither): dimmed "✕" strike,
  //                     NOT clickable, tooltip "not available on this host".
  let disabledSet = $derived(new Set(node.disabled ?? []));

  function effHas(feature) {
    if (feature === "playback") return !!caps.playback;
    if (feature === "opus") return (caps.codecs ?? []).includes("opus");
    if (feature === "input") return (caps.sources ?? []).includes("input");
    return false;
  }

  // state(feature) → "on" | "off" | "unavailable"
  function featState(feature) {
    if (disabledSet.has(feature)) return "off";
    return effHas(feature) ? "on" : "unavailable";
  }

  // The state's glyph + accessible tooltip.
  function featGlyph(st) {
    if (st === "on") return "●";
    if (st === "off") return "○";
    return "✕";
  }
  function featTitle(feature, st) {
    if (st === "unavailable") return `${feature}: not available on this host`;
    if (st === "off") return `${feature}: disabled — click to enable`;
    return `${feature}: enabled — click to disable`;
  }

  // Toggle a feature's disabled membership and PATCH the new list. Unavailable
  // chips are inert (probed off — nothing to toggle).
  function toggleFeature(feature) {
    const st = featState(feature);
    if (st === "unavailable") return;
    const next = new Set(disabledSet);
    if (st === "off") next.delete(feature);
    else next.add(feature);
    setDisabled(node.id, [...next]).catch(() => {});
  }

  let features = ["playback", "opus", "input"];
</script>

<div class="noderow card" class:stale={!node.alive}>
  <div class="row wrap between">
    <div class="row">
      <span class="dot {node.alive ? 'alive' : 'dead'}"></span>
      <EditableText
        value={node.name}
        onsave={(n) => renameNode(node.id, n)}
        placeholder="(unnamed)"
      />
      {#if isSelf}<span class="chip">this node</span>{/if}
      <span class="node-id small" title={node.id}>{shortId(node.id)}</span>
    </div>
    <span class="muted small">
      {node.alive ? relTime(node.lastSeen) : "offline"}
      {#if node.stale}<span class="offline"> · stale</span>{/if}
    </span>
  </div>

  <div class="row wrap small muted netinfo">
    {cidrList(node.addrs)}
  </div>

  {#if portList}
    <div class="row wrap small muted netinfo">
      {portList}
    </div>
  {/if}

  <div class="row wrap feature-row">
    <span class="muted small feat-label">features</span>
    {#each features as f (f)}
      {@const st = featState(f)}
      <button
        type="button"
        class="chip feat {st}"
        disabled={st === "unavailable"}
        aria-pressed={st === "on"}
        title={featTitle(f, st)}
        onclick={() => toggleFeature(f)}
      >
        <span class="glyph" aria-hidden="true">{featGlyph(st)}</span>{f}
      </button>
    {/each}
  </div>

  {#if (caps.codecs ?? []).filter((c) => c !== "opus").length || (caps.formats ?? []).length}
    <div class="row wrap format-row">
      <span class="muted small feat-label">formats</span>
      {#each (caps.codecs ?? []).filter((c) => c !== "opus") as c}
        <span class="chip plain">{c}</span>
      {/each}
      {#each caps.formats ?? [] as f}<span class="chip plain">{f}</span>{/each}
    </div>
  {/if}

  <div class="row wrap">
    <span class="muted small">vol</span>
    <VolumeSlider value={node.volume} onchange={(v) => setVolume(node.id, v)} />
    <span class="spacer"></span>
    <div class="delay">
      <label class="row small muted">
        output delay (ms)
        <input
          type="number"
          min="-500"
          max="500"
          bind:value={delayDraft}
          onblur={commitDelay}
          onkeydown={onkey}
          style="width: 70px;"
        />
      </label>
      <div class="hint">
        compensates fixed device latency; causes a brief local restart
      </div>
    </div>
  </div>

  <div class="row wrap">
    {#if outputDevices.length > 0}
      <label class="row small muted device">
        output device
        <select value={outputDevice} onchange={onDeviceChange}>
          {#each outputDevices as d (d.id)}
            <option value={d.id}>{d.desc} ({d.id})</option>
          {/each}
        </select>
      </label>
    {/if}
    <button class="small" title="play a 1s test tone on this node's output"
      onclick={() => testTone(node.id)}>♪ test tone</button>
  </div>
</div>

<style>
  /* vertical rhythm between the stacked rows inside a node card */
  .noderow {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
  }
  /* address + port lines: breathing room and comfortable wrap spacing */
  .netinfo {
    padding: 2px 0;
    line-height: 1.6;
  }
  .delay {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 2px;
  }
  .delay .hint {
    text-align: right;
  }
  .device {
    gap: 6px;
  }
  .device select {
    max-width: 16rem;
  }
  /* the two chip rows: a small leading label, then the chips */
  .feature-row,
  .format-row {
    align-items: center;
    gap: 6px;
  }
  .feat-label {
    min-width: 3.6rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    font-size: 0.7rem;
    opacity: 0.7;
  }

  /* tri-state feature chips (D40): three UNMISTAKABLE states. */
  .chip.feat {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font: inherit;
    border: 1px solid transparent;
    border-radius: 999px;
    padding: 2px 10px;
  }
  .chip.feat .glyph {
    font-size: 0.8em;
    line-height: 1;
  }

  /* ON: available + enabled — solid green accent, ● , clickable. */
  .chip.feat.on {
    cursor: pointer;
    background: #15803d;
    border-color: #15803d;
    color: #fff;
  }
  .chip.feat.on:hover {
    background: #166534;
  }

  /* OFF: available but disabled — outlined amber, ○ , clickable. */
  .chip.feat.off {
    cursor: pointer;
    background: transparent;
    border-color: #b45309;
    color: #b45309;
  }
  .chip.feat.off:hover {
    background: rgba(180, 83, 9, 0.12);
  }

  /* UNAVAILABLE: not probed on this host — dimmed + strike, ✕ , NOT clickable. */
  .chip.feat.unavailable {
    cursor: not-allowed;
    background: transparent;
    border-color: var(--border, #ccc);
    color: var(--muted, #888);
    opacity: 0.5;
    text-decoration: line-through;
  }

  /* passive format chips: plain, never togglable, default cursor. */
  .chip.plain {
    cursor: default;
  }
</style>
