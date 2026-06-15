<script>
  // One node in the Nodes table (J arch §4): editable name, id, addrs, caps,
  // liveness, volume, output-delay calibration.
  import { shortId, cidrList, relTime, ports } from "../lib/fmt.js";
  import {
    nodeRename,
    nodeSetVolume,
    nodeSetOutputDelay,
    nodeSetChannel,
    setOutputDevice,
    setDisabled,
    testTone,
    forgetNode,
  } from "../lib/api.js";
  import EditableText from "./EditableText.svelte";
  import VolumeSlider from "./VolumeSlider.svelte";
  import SpotifyEndpoints from "./SpotifyEndpoints.svelte";
  import { playbackStats } from "../lib/stats.svelte.js";

  let { node, self, snapshot } = $props();

  // Per-node sync-health telemetry, collected by the master from the STATUS control
  // payload (§7, D19). Present for playback members the master tracks; greyed when
  // stale (no STATUS in >3s). The coherence-relevant numbers are offset & phase.
  let stat = $derived(playbackStats.byId[node.id]);
  let statStale = $derived(stat ? stat.ageMs > 3000 : false);
  const fmtMs = (ns) => (ns >= 0 ? "+" : "") + (ns / 1e6).toFixed(2) + " ms";
  const fmtUs = (ns) => (ns >= 0 ? "+" : "") + (ns / 1e3).toFixed(0) + " µs";

  let isSelf = $derived(node.id === self.id);
  // A dead, non-self node can be forgotten: deleted from the cluster and purged
  // from every config that referenced it. Playback nodes count too (they go dead
  // when their mDNS advert stops). Guarded by a confirm — it's destructive.
  let canForget = $derived(!isSelf && !node.alive);
  function onForget() {
    const label = node.name || shortId(node.id);
    if (!confirm(`Delete node "${label}" from the cluster?\n\nIt will be removed everywhere and purged from group/Spotify references. If it comes back online it will reappear.`))
      return;
    forgetNode(node.id).catch(() => {});
  }
  let caps = $derived(node.capabilities || {});
  let canSpotify = $derived((caps.sources ?? []).includes("spotify"));
  let portList = $derived(ports(node));

  // Output-delay slider (D36), wired exactly like VolumeSlider: the held draft
  // tracks the thumb while dragging; a fresh snapshot re-syncs once released.
  // Range 0..150 ms, step 5, debounced ~200ms PATCH {outputDelayMs}. The old
  // number input reset to 0 mid-edit; this controlled pattern fixes it.
  let delayDragging = $state(false);
  let delayMs = $state(0);
  $effect(() => {
    const v = node.outputDelayMs ?? 0;
    if (!delayDragging) delayMs = v;
  });

  // Channel mode (stereo / L / R, dual-mono): a pending draft the operator commits
  // with ATTACH; RESET reverts to stereo. The APPLIED value is node.channel; the draft
  // adopts it (via the effect) only while not mid-edit, so a 1 Hz snapshot doesn't
  // clobber an in-progress pick — same shape as the hw-delay draft above.
  let channelSel = $state("stereo");
  let channelEditing = $state(false);
  let appliedChannel = $derived(node.channel || "stereo");
  let channelDirty = $derived(channelSel !== appliedChannel);
  $effect(() => {
    const v = node.channel || "stereo";
    if (!channelEditing) channelSel = v;
  });
  function channelAttach() {
    channelEditing = false;
    nodeSetChannel(node, channelSel).catch(() => {});
  }
  function channelReset() {
    channelEditing = false;
    channelSel = "stereo";
    nodeSetChannel(node, "stereo").catch(() => {});
  }

  let delayTimer = null;
  function fireDelay() {
    if (delayTimer) {
      clearTimeout(delayTimer);
      delayTimer = null;
    }
    nodeSetOutputDelay(node, delayMs).catch(() => {});
  }
  function onDelayInput(e) {
    delayDragging = true;
    delayMs = Number(e.target.value);
    if (delayTimer) clearTimeout(delayTimer);
    delayTimer = setTimeout(() => {
      delayTimer = null;
      nodeSetOutputDelay(node, delayMs).catch(() => {});
    }, 200);
  }
  function onDelayCommit() {
    fireDelay();
    delayDragging = false;
  }

  // output-device selection (D37). List comes from the node's enumeration; the
  // <select> is hidden when empty (no ALSA on that host).
  let outputDevices = $derived(node.outputDevices ?? []);
  let outputDevice = $derived(node.outputDevice ?? "default");
  // the CHOSEN sink backend actually playing audio (alsa/exec/null, §8.5).
  let outputBackend = $derived(node.outputBackend ?? "");

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
        onsave={(n) => nodeRename(node, n)}
        placeholder="(unnamed)"
      />
      {#if isSelf}<span class="chip">this node</span>{/if}
      <span class="node-id small" title={node.id}>{shortId(node.id)}</span>
      {#if node.appVersion}
        <span class="chip plain ver" title="build version (from mDNS advert)">{node.appVersion}</span>
      {/if}
    </div>
    <span class="row small">
      <span class="muted">
        {#if node.alive}
          {relTime(node.lastSeen)}
        {:else}
          offline{#if node.stale}<span class="offline"> · stale</span>{/if}
        {/if}
      </span>
      {#if canForget}
        <button
          type="button"
          class="chip forget"
          title="Delete this offline node from the cluster"
          onclick={onForget}
        >
          delete
        </button>
      {/if}
    </span>
  </div>

  <section class="node-section">
    <h4 class="node-section-h">addresses</h4>
    <div class="row wrap small muted netinfo">
      {cidrList(node.addrs)}
    </div>
    {#if portList}
      <div class="row wrap small muted netinfo">
        {portList}
      </div>
    {/if}
  </section>

  <section class="node-section">
    <h4 class="node-section-h">features</h4>
    <div class="row wrap feature-row">
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
      {#if canSpotify}
        <span class="chip spotify" title="this node runs go-librespot (Spotify Connect)">spotify</span>
      {/if}
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
  </section>

  {#if stat}
    <section class="node-section sync-health" class:stale={statStale}>
      <h4 class="node-section-h">sync health{statStale ? " · stale" : ""}</h4>
      <div class="sync-metrics">
        <div class="sm-cell" title="clock offset to master (master − local)"><span class="sm-label">offset</span><span class="sm-val">{fmtMs(stat.offsetNs)}</span></div>
        <div class="sm-cell" title="round-trip time of the clock sync"><span class="sm-label">rtt</span><span class="sm-val">{fmtMs(stat.rttNs)}</span></div>
        <div class="sm-cell" title="servo rate correction (ppm)"><span class="sm-label">drift</span><span class="sm-val">{stat.ratePPM.toFixed(1)} ppm</span></div>
        <div class="sm-cell" title="playout phase error vs the smoothed model"><span class="sm-label">phase</span><span class="sm-val">{fmtUs(stat.phaseErrNs)}</span></div>
        {#if stat.deviceDelayNs}<div class="sm-cell" title="measured output (device) latency"><span class="sm-label">dev</span><span class="sm-val">{fmtMs(stat.deviceDelayNs)}</span></div>{/if}
        <div class="sm-cell" title="jitter-buffer depth (frames)"><span class="sm-label">buf</span><span class="sm-val">{stat.buffered}f</span></div>
        <div class="sm-cell" title="silent frames inserted for gaps (underrun proxy)"><span class="sm-label">silence</span><span class="sm-val">{stat.silence}</span></div>
        <div class="sm-cell" title="frames dropped (arrived past deadline)"><span class="sm-label">late</span><span class="sm-val">{stat.late}</span></div>
        <div class="sm-cell" title="cumulative samples the rate-servo duplicated into the output (realized correction, not commanded ppm)"><span class="sm-label">inj</span><span class="sm-val">{stat.samplesInjected} ({(stat.samplesInjected / 48).toFixed(0)} ms)</span></div>
        <div class="sm-cell" title="cumulative samples the rate-servo dropped from the output (realized correction, not commanded ppm)"><span class="sm-label">drop</span><span class="sm-val">{stat.samplesDropped} ({(stat.samplesDropped / 48).toFixed(0)} ms)</span></div>
        <div class="sm-cell" class:sm-ok={stat.calibrated} class:sm-bad={!stat.calibrated} title="servo setpoint captured (device-queue depth stable)"><span class="sm-label">calibrated</span><span class="sm-val">{stat.calibrated ? "✓" : "✗"}</span></div>
      </div>
    </section>
  {/if}

  {#if canSpotify}
    <section class="node-section">
      <h4 class="node-section-h">spotify endpoints</h4>
      <SpotifyEndpoints {node} {snapshot} />
    </section>
  {/if}

  <section class="node-section">
    <h4 class="node-section-h">settings</h4>
    <!-- vol + hw-delay only for nodes that actually play (a --role master node
         has playback:false — no player, no volume/delay). The two sliders share a
         fixed-width label column so their tracks line up. -->
    {#if caps.playback}
      <div class="setting-row">
        <span class="muted small setting-label">vol</span>
        <VolumeSlider value={node.volume} onchange={(v) => nodeSetVolume(node, v)} />
      </div>
      <div class="setting-row">
        <span class="muted small setting-label" title="compensates fixed device latency; causes a brief local restart">hw delay</span>
        <span class="vol">
          <input
            type="range"
            min="0"
            max="150"
            step="5"
            value={delayMs}
            oninput={onDelayInput}
            onchange={onDelayCommit}
            onpointerup={onDelayCommit}
            aria-label="hw delay (ms)"
          />
          <span class="pct small">{delayMs} ms</span>
        </span>
      </div>
      <div class="setting-row">
        <span class="muted small setting-label" title="play a single channel as dual-mono (the chosen channel on both speakers)">channel</span>
        <span class="row small">
          <select bind:value={channelSel} onchange={() => (channelEditing = true)} aria-label="channel mode">
            <option value="stereo">Stereo</option>
            <option value="L">L (left)</option>
            <option value="R">R (right)</option>
          </select>
          <button class="btn small" onclick={channelAttach}
            disabled={!channelDirty}
            title="apply the selected channel">attach</button>
          <button class="btn small" onclick={channelReset}
            disabled={appliedChannel === "stereo"}
            title="revert to stereo">reset</button>
          {#if appliedChannel !== "stereo"}<span class="muted small" title="currently applied">· {appliedChannel}</span>{/if}
        </span>
      </div>
    {/if}

    <div class="row wrap">
      {#if outputBackend}
        <span class="small muted sink" title="the sink backend actually playing audio on this node">
          sink <strong>{outputBackend}</strong>
        </span>
      {/if}
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
      <button class="btn small" title="play a 1s test tone on this node's output"
        onclick={() => testTone(node.id)}>♪ test tone</button>
    </div>
  </section>
</div>

<style>
  .noderow {
    flex-direction: column;
    align-items: stretch;
    /* sections own their own spacing around the divider (gap would add to it). */
    gap: 0;
  }
  /* each labeled section is set off by a rule (addresses / features / spotify /
     settings). Every divider has the SAME space above (margin) and below
     (padding): 12px above the line, 16px below. */
  .node-section {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-top: 14px;
    padding-top: 18px;
    border-top: 1px solid var(--border);
  }
  .sync-health.stale {
    opacity: 0.45;
  }

  /* sync health as a structured label/value grid (ensemble-design): a 4-column
     grid of cells, each a muted uppercase label over a mono tabular value, rather
     than a wrap of label+value pills. sm-ok / sm-bad tint the value by health. */
  .sync-metrics {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 10px 16px;
    padding: 6px 0 2px;
  }
  .sm-cell {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .sm-label {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--muted);
  }
  .sm-val {
    font-size: 12px;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-variant-numeric: tabular-nums;
    color: var(--fg);
  }
  .sm-cell.sm-ok .sm-val {
    color: var(--ok);
  }
  .sm-cell.sm-bad .sm-val {
    color: var(--danger);
  }
  .node-section-h {
    margin: 0;
    font-size: 0.7rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--muted);
  }
  /* address + port lines: breathing room and comfortable wrap spacing */
  .netinfo {
    padding: 6px 0;
    line-height: 1.6;
  }
  /* a touch more separation around the whole netinfo block */
  .netinfo:first-of-type {
    margin-top: 2px;
  }
  .netinfo + .netinfo {
    margin-top: -2px;
  }
  /* vol + hw-delay: a fixed label column so both slider tracks start at the same
     x and span the same width; the value sits in a fixed column on the right. */
  .setting-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding-block: 4px;
  }
  .setting-label {
    flex: 0 0 4.5rem;
  }
  .setting-row :global(.vol) {
    flex: 1 1 auto;
    min-width: 0;
  }
  .setting-row :global(.vol input[type="range"]) {
    flex: 1 1 auto;
    min-width: 0;
    width: auto;
  }
  .setting-row :global(.vol .pct) {
    width: 3.2rem;
  }
  .device {
    gap: 6px;
  }
  .device select {
    max-width: 16rem;
  }
  .sink {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .sink strong {
    color: var(--fg);
    font-weight: 600;
  }
  /* the two chip rows: a small leading label, then the chips */
  .feature-row,
  .format-row {
    align-items: center;
    gap: 8px;
    padding: 6px 0;
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
    /* match the passive FORMATS chips exactly (app.css .chip: 11px / 0 8px),
       with button UA defaults zeroed — the pills must not read "button". */
    appearance: none;
    display: inline-flex;
    align-items: center;
    gap: 3px;
    margin: 0;
    min-height: 0;
    font-size: 11px;
    line-height: 1.5;
    border: 1px solid transparent;
    border-radius: 10px;
    padding: 0 8px;
  }
  .chip.feat .glyph {
    font-size: 8px;
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

  /* presence badge: this node runs go-librespot (not a toggle), Spotify green. */
  .chip.spotify {
    cursor: default;
    background: #1db954;
    border: 1px solid #1db954;
    color: #04210f;
    font-weight: 600;
  }

  /* delete an offline node — destructive, so it reads red and only appears for
     dead nodes (see canForget). */
  .chip.forget {
    cursor: pointer;
    background: transparent;
    border: 1px solid color-mix(in srgb, #e5484d 50%, transparent);
    color: #e5484d;
  }
  .chip.forget:hover {
    background: #e5484d;
    color: #fff;
    border-color: #e5484d;
  }
</style>
