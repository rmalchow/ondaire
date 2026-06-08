<script>
  // Playback row for a group (J arch §4, D39): a single fixed-shape row — the
  // currently-playing track (ellipsised, "idle" when stopped) on the left, a
  // play/pause toggle + stop on the right. The controls keep the SAME width in
  // every state; only their icon/enabled state changes with availability.
  import { position } from "../lib/fmt.js";
  import { stop, pause, resume } from "../lib/api.js";

  let { group } = $props();

  let pb = $derived(group.playback || { state: "idle" });
  let playing = $derived(pb.state === "playing");
  let paused = $derived(pb.state === "paused");
  let active = $derived(playing || paused);

  // a friendly one-line name for the source uri.
  let track = $derived(friendlyTrack(pb.uri));
  function friendlyTrack(uri) {
    if (!uri) return "";
    if (uri.startsWith("input:")) return "line-in";
    if (uri.startsWith("file:")) {
      const p = uri.slice(5);
      return p.split("/").pop() || p;
    }
    return uri; // http(s):// stream — show the url
  }

  async function ontoggle() {
    try {
      await (playing ? pause(group.master) : resume(group.master));
    } catch {
      // toast shown by api.js
    }
  }
  async function onstop() {
    try {
      await stop(group.master);
    } catch {
      // toast shown by api.js
    }
  }
</script>

<div class="playbar">
  <div class="now">
    {#if active}
      <span class="state {paused ? 'paused' : 'playing'}"
        >{paused ? "paused" : "playing"}</span
      >
      <span class="track" title={pb.uri}>{track}</span>
      <span class="pos small">{position(pb.positionSec)}</span>
    {:else}
      <span class="muted idle">idle</span>
    {/if}
  </div>

  <div class="controls">
    <button
      class="btn ctl"
      disabled={!active}
      onclick={ontoggle}
      title={playing ? "pause" : "resume"}
      aria-label={playing ? "pause" : "resume"}
    >
      {playing ? "⏸" : "▶"}
    </button>
    <button
      class="btn btn-danger ctl"
      disabled={!active}
      onclick={onstop}
      title="stop"
      aria-label="stop"
    >
      ■
    </button>
  </div>
</div>

<style>
  .playbar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 4px 0 8px;
  }

  /* left: fills the row, ellipsises the track so the controls never move */
  .now {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .now .track {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .now .pos {
    flex: 0 0 auto;
    font-variant-numeric: tabular-nums;
    color: var(--muted);
  }
  .now .state {
    flex: 0 0 auto;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--ok);
  }
  .now .state.paused {
    color: #f59e0b;
  }
  .now .idle {
    font-style: italic;
  }

  /* right: two equal-width controls, identical footprint in every state */
  .controls {
    flex: 0 0 auto;
    display: flex;
    gap: 6px;
  }
  .controls .ctl {
    width: 40px;
    padding: 4px 0;
    text-align: center;
    line-height: 1;
  }
  .controls .ctl:disabled {
    opacity: 0.4;
    cursor: default;
  }
</style>
