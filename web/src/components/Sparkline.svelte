<script>
  // A minimal inline-SVG sparkline. Stroke + fill inherit `currentColor`, so the
  // parent tints it by setting `color` (e.g. an ok/warn/danger class). Width is
  // fluid (the SVG stretches via preserveAspectRatio=none; the stroke stays 1px
  // thanks to vector-effect=non-scaling-stroke). `signed` folds 0 into the range
  // and draws a faint zero baseline — for values that swing about 0 (offset/phase).
  let { values = [], height = 34, signed = false } = $props();

  const W = 100;

  let pts = $derived(values.filter((v) => Number.isFinite(v)));
  let view = $derived.by(() => {
    const n = pts.length;
    if (n < 2) return null;
    const pad = 4;
    let lo = Math.min(...pts);
    let hi = Math.max(...pts);
    if (signed) {
      const m = Math.max(Math.abs(lo), Math.abs(hi), 1e-9);
      lo = -m;
      hi = m;
    }
    if (hi - lo < 1e-9) {
      hi += 1;
      lo -= 1;
    }
    const span = hi - lo;
    const x = (i) => (i / (n - 1)) * W;
    const y = (v) => pad + (1 - (v - lo) / span) * (height - 2 * pad);
    const line = pts.map((v, i) => `${x(i).toFixed(2)},${y(v).toFixed(2)}`);
    const area = `0,${height} ${line.join(" ")} ${W},${height}`;
    const zeroY = lo <= 0 && hi >= 0 ? y(0) : null;
    return {
      line: line.join(" "),
      area,
      zeroY,
      lastX: x(n - 1),
      lastY: y(pts[n - 1]),
    };
  });
</script>

{#if view}
  <svg
    class="spark"
    viewBox="0 0 {W} {height}"
    preserveAspectRatio="none"
    aria-hidden="true"
  >
    <defs>
      <linearGradient id="sparkfade" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="currentColor" stop-opacity="0.22" />
        <stop offset="1" stop-color="currentColor" stop-opacity="0" />
      </linearGradient>
    </defs>
    {#if view.zeroY !== null}
      <line
        class="zero"
        x1="0"
        x2={W}
        y1={view.zeroY}
        y2={view.zeroY}
        vector-effect="non-scaling-stroke"
      />
    {/if}
    <polygon class="fill" points={view.area} fill="url(#sparkfade)" />
    <polyline
      class="stroke"
      points={view.line}
      fill="none"
      vector-effect="non-scaling-stroke"
    />
    <circle class="head" cx={view.lastX} cy={view.lastY} r="1.6" />
  </svg>
{:else}
  <div class="spark-empty" aria-hidden="true"></div>
{/if}

<style>
  .spark {
    display: block;
    width: 100%;
    height: var(--spark-h, 34px);
    color: inherit;
    overflow: visible;
  }
  .stroke {
    stroke: currentColor;
    stroke-width: 1.5;
    stroke-linejoin: round;
    stroke-linecap: round;
  }
  .zero {
    stroke: var(--muted);
    stroke-width: 1;
    stroke-dasharray: 2 3;
    opacity: 0.4;
  }
  .head {
    fill: currentColor;
  }
  .spark-empty {
    height: var(--spark-h, 34px);
    border-radius: 6px;
    background: repeating-linear-gradient(
      -45deg,
      color-mix(in srgb, var(--muted) 12%, transparent) 0 6px,
      transparent 6px 12px
    );
    opacity: 0.5;
  }
</style>
