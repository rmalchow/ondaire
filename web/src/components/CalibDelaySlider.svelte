<script>
  // One node's output-delay control for calibration: CENTERED at 0, bidirectional
  // (you don't know which speaker is late), WIDE range, 1 ms steps. Same held-draft
  // + debounce shape as the NodeRow delay slider; writes the node-owned
  // OutputDelayMs (persisted). The parent row supplies the name + mute control.
  import { nodeSetOutputDelay } from "../lib/api.js";

  let { node, label = "" } = $props();

  let dragging = $state(false);
  let ms = $state(0);
  $effect(() => {
    const v = node.outputDelayMs ?? 0;
    if (!dragging) ms = v; // re-sync from snapshot once released (and after re-center)
  });

  let timer = null;
  function onInput(e) {
    dragging = true;
    ms = Number(e.target.value);
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      nodeSetOutputDelay(node, ms).catch(() => {});
    }, 200);
  }
  function onCommit() {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    nodeSetOutputDelay(node, ms).catch(() => {});
    dragging = false;
  }
  function zero() {
    dragging = false;
    ms = 0;
    nodeSetOutputDelay(node, 0).catch(() => {});
  }
</script>

<input
  class="cal-range"
  type="range"
  min="-200"
  max="200"
  step="1"
  value={ms}
  oninput={onInput}
  onchange={onCommit}
  onpointerup={onCommit}
  aria-label="output delay for {label}"
/>
<span class="cal-val">{ms > 0 ? "+" : ""}{ms} ms</span>
<button class="cal-zero" type="button" onclick={zero} title="reset to 0 ms">0</button>

<style>
  .cal-range {
    flex: 1;
    min-width: 0;
  }
  .cal-val {
    min-width: 4.4em;
    text-align: right;
    color: var(--muted);
    font-variant-numeric: tabular-nums;
  }
  .cal-zero {
    font: inherit;
    cursor: pointer;
    color: inherit;
    background: transparent;
    border: 1px solid rgba(255, 255, 255, 0.15);
    border-radius: 4px;
    padding: 1px 7px;
  }
</style>
