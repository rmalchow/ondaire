<script>
  // 0–100% range → debounced setVolume (J arch §4 / D35). The held pct tracks
  // the thumb while dragging; a fresh snapshot re-syncs once released.
  let { value, onchange } = $props();

  let dragging = $state(false);
  let pct = $state(0);

  // Re-sync to the server truth when a new snapshot arrives and not dragging.
  $effect(() => {
    const v = Math.round((value || 0) * 100);
    if (!dragging) pct = v;
  });

  let timer = null;
  function fire() {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    onchange(pct / 100).catch(() => {});
  }

  function oninput(e) {
    dragging = true;
    pct = Number(e.target.value);
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      onchange(pct / 100).catch(() => {});
    }, 150);
  }

  function oncommit() {
    // trailing call on pointerup/change so the final position always lands.
    fire();
    dragging = false;
  }
</script>

<span class="vol">
  <input
    type="range"
    min="0"
    max="100"
    value={pct}
    {oninput}
    onchange={oncommit}
    onpointerup={oncommit}
    aria-label="volume"
  />
  <span class="pct small">{pct}%</span>
</span>
