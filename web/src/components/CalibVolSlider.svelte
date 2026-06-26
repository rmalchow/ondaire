<script>
  // Narrow per-node volume slider for calibration — to balance/isolate speakers
  // while comparing. Deliberately small: the delay slider is the primary control.
  import { nodeSetVolume } from "../lib/api.js";

  let { node } = $props();

  let dragging = $state(false);
  let pct = $state(0);
  $effect(() => {
    const v = Math.round((node.volume ?? 0) * 100);
    if (!dragging) pct = v;
  });

  let timer = null;
  function onInput(e) {
    dragging = true;
    pct = Number(e.target.value);
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      nodeSetVolume(node, pct / 100).catch(() => {});
    }, 150);
  }
  function onCommit() {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    nodeSetVolume(node, pct / 100).catch(() => {});
    dragging = false;
  }
</script>

<input
  class="cal-vol"
  type="range"
  min="0"
  max="100"
  value={pct}
  oninput={onInput}
  onchange={onCommit}
  onpointerup={onCommit}
  aria-label="volume"
  title="volume {pct}%"
/>

<style>
  .cal-vol {
    flex: none;
    width: 84px;
  }
</style>
