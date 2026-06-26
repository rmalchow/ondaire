<script>
  // Playback row for a group (J arch §4, D39): a fixed-height band — a state pill,
  // the source-type icon + currently-playing track, position, and play/pause + stop
  // on the right. When active it tints (accent band) so the playing room is obvious
  // at a glance; idle keeps the SAME footprint, just emptier (no reflow either way).
  import { position } from "../lib/fmt.js";
  import { createPlayClock, reconcile, sample, markSeek } from "../lib/playclock.js";
  import {
    stop,
    pause,
    resume,
    next,
    queueRemove,
    queuePlay,
    getQueue,
    seek,
    base,
  } from "../lib/api.js";

  let { group, expanded = false, streamPresets = [] } = $props();

  let pb = $derived(group.playback || { state: "idle" });
  let playing = $derived(pb.state === "playing");
  let paused = $derived(pb.state === "paused");
  let active = $derived(playing || paused);

  // The UPCOMING queue. Only its length + a change marker (queueRev) ride the
  // gossiped playback record — a big queue would blow memberlist's UDP packet and
  // stall propagation — so the items themselves are pulled live from the master,
  // proxied, whenever the marker (or now-playing track) moves. `next` is enabled
  // only while actually playing AND something is queued behind it.
  let queueLen = $derived(pb.queueLen || 0);
  let canNext = $derived(playing && queueLen > 0);

  // fetched upcoming items (only while the card is expanded and there's a queue).
  let queue = $state([]);
  let lastQueueKey = ""; // guards refetch: only re-pull when the queue actually moved
  $effect(() => {
    // Reads below register as effect dependencies; position-only heartbeat ticks
    // (not in the key) don't re-pull.
    const key = `${group.master}|${pb.queueRev || 0}|${queueLen}|${pb.uri}|${expanded}|${active}`;
    if (key === lastQueueKey) return;
    lastQueueKey = key;
    if (!expanded || !active || queueLen <= 0) {
      queue = [];
      return;
    }
    const master = group.master;
    getQueue(master)
      .then((list) => {
        if (lastQueueKey === key) queue = Array.isArray(list) ? list : [];
      })
      .catch(() => {
        if (lastQueueKey === key) queue = [];
      });
  });

  // saved name of the stream preset currently playing (stream:<id>), or "".
  let presetName = $derived(streamPresetName(pb.uri));
  // a friendly one-line name + a type glyph for the source uri.
  let track = $derived(friendlyTrack(pb.uri, presetName));
  let icon = $derived(iconFor(pb.uri));

  // now-playing metadata (the D57 source channel): title/artist/album/cover art.
  // Falls back to the URI-derived label when the source supplies none (line-in).
  let meta = $derived(pb.metadata || null);
  let title = $derived(meta && meta.title ? meta.title : track);
  let subtitle = $derived.by(() => {
    const parts = meta ? [meta.artist, meta.album] : [];
    // A stream: keep the saved station name visible even while a song plays.
    if (presetName && (meta?.title || "") !== presetName) parts.push(presetName);
    return parts.filter(Boolean).join(" · ");
  });
  // track length for the transport bar (seconds); 0 = unknown (live/line-in →
  // the bar shows elapsed time only, no scrub track). Files report it from the
  // decoder, Spotify from go-librespot.
  let durationSec = $derived(meta && meta.durationSec ? meta.durationSec : 0);

  // Cover art for the now-playing track (D57), shown in the bar's left slot.
  // Spotify carries an absolute artUrl we load directly; a file carries no URL —
  // only a hasArt hint — so we fetch the bytes from the MASTER's /cover endpoint
  // (proxied like the queue), keyed by the track URI so it changes track-to-track
  // and caches per track. When there's no art (or the fetch fails) the slot falls
  // back to a CSS-only placeholder, keeping the bar's footprint identical.
  let coverSrc = $derived.by(() => {
    if (!active || !meta || !meta.hasArt) return "";
    if (meta.artUrl) return meta.artUrl; // spotify: direct remote URL
    if (pb.uri) return base(group.master) + "/cover?uri=" + encodeURIComponent(pb.uri);
    return "";
  });
  let artFailed = $state(false);
  let lastCover = "";
  $effect(() => {
    // reset the error latch whenever the source changes (new track / room).
    if (coverSrc !== lastCover) {
      lastCover = coverSrc;
      artFailed = false;
    }
  });
  let showCover = $derived(!!coverSrc && !artFailed);

  // Smooth position: the server reports positionSec only ~every 5 s (group
  // heartbeat) and a little stale, but position is realtime between events — so a
  // local clock (lib/playclock) free-runs at 1x, gently slews toward each heartbeat,
  // and hard-snaps only on a real discontinuity (track change, resume, seek). The
  // clock object is PLAIN (non-reactive) so the ticker writing displayPos can't feed
  // back into the reconcile effect; displayPos ($state) is the only reactive output.
  let displayPos = $state(0);
  const clock = createPlayClock();
  $effect(() => {
    reconcile(clock, {
      positionSec: pb.positionSec || 0, // tracked deps: position, uri, state
      uri: pb.uri || "",
      playing,
      nowMs: performance.now(),
    });
    displayPos = sample(clock, performance.now(), durationSec);
  });
  $effect(() => {
    if (!playing) return; // frozen (paused/idle); the reconcile effect set displayPos
    const id = setInterval(() => {
      displayPos = sample(clock, performance.now(), durationSec);
    }, 250);
    return () => clearInterval(id);
  });

  // Scrubbing: enabled only when the source reports seekable (file queue) and we
  // know its length. While dragging we show the dragged value (the ticker keeps
  // running underneath but the bar reflects the user); on release we optimistically
  // jump the clock and POST the seek.
  let seekable = $derived(!!pb.seekable && durationSec > 0);
  let dragging = $state(false);
  let dragValue = $state(0);
  function onSeekInput(e) {
    dragging = true;
    dragValue = Number(e.target.value);
  }
  function onSeekCommit(e) {
    const v = Number(e.target.value);
    dragging = false;
    markSeek(clock, v, performance.now());
    displayPos = v;
    seek(group.master, v).catch(() => {});
  }

  // resolve a stream:<id> URI to its saved preset name (cluster-wide list).
  function streamPresetName(uri) {
    if (!uri || !uri.startsWith("stream:")) return "";
    const id = uri.slice(7);
    const p = (streamPresets || []).find((s) => s.id === id);
    return p ? p.name : "";
  }
  function friendlyTrack(uri, presetName) {
    if (!uri) return "";
    if (uri.startsWith("spotify:")) return "Spotify";
    if (uri.startsWith("input:")) return "line-in";
    if (uri.startsWith("stream:")) return presetName || "stream";
    if (uri.startsWith("file:")) {
      const p = uri.slice(5);
      return p.split("/").pop() || p;
    }
    return uri; // http(s):// stream — show the url
  }
  function iconFor(uri) {
    if (!uri) return "";
    if (uri.startsWith("spotify:")) return "🟢";
    if (uri.startsWith("input:")) return "🎙";
    if (uri.startsWith("http") || uri.startsWith("stream:")) return "📻";
    return "♪"; // file
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
  async function onnext() {
    try {
      await next(group.master);
    } catch {
      // toast shown by api.js
    }
  }
  async function onremove(index, uri) {
    try {
      await queueRemove(group.master, index, uri);
    } catch {
      // toast shown by api.js
    }
  }
  // clicking a queue item plays it now: the current track is dropped, the clicked
  // track jumps to the front and starts playing (gapless front-switch).
  async function onplay(index, uri) {
    try {
      await queuePlay(group.master, index, uri);
    } catch {
      // toast shown by api.js
    }
  }

  // a queue entry's display label: tag title (+ artist) when present, else the
  // URI-derived (filename) fallback — same as the now-playing bar.
  function queueTitle(item) {
    if (item.metadata && item.metadata.title) return item.metadata.title;
    return friendlyTrack(item.uri);
  }
  function queueSub(item) {
    return item.metadata ? item.metadata.artist || "" : "";
  }
</script>

<!-- One grid band per room: a fixed cover slot on the left, then the state pill +
     now-playing + transport controls, the scrubber, and (selected, playing) the
     queue. Idle, playing-no-queue, and playing-with-queue all keep the SAME left
     slot + footprint — only the right column fills in, so cards never reflow. -->
<div class="playbar" class:active class:idle={!active}>
  {#if showCover}
    <!-- the sharp art over a dimmed/blurred/cropped copy of itself, so any
         letterboxing reads as a soft tint of the cover. Both <img>s hit one URL →
         one fetch; onerror collapses to the placeholder. -->
    <div class="cover">
      <img class="cover-bg" src={coverSrc} alt="" aria-hidden="true" />
      <img class="cover-art" src={coverSrc} alt="cover art" onerror={() => (artFailed = true)} />
    </div>
  {:else}
    <!-- reserved cover slot: a CSS-only placeholder when there's no art (idle,
         line-in, or a file with no embedded cover) keeps the footprint identical. -->
    <div class="cover-placeholder" aria-hidden="true"></div>
  {/if}

  <div class="pb-row">
    <span class="state-pill" class:playing class:paused>
      {active ? (paused ? "paused" : "playing") : "idle"}
    </span>

    <div class="now">
      {#if active}
        <!-- a small source-type glyph identifies the source at a glance. -->
        {#if icon}
          <span class="icon">{icon}</span>
        {/if}
        <span class="meta" title={pb.uri}>
          <span class="track">{title}</span>
          {#if subtitle}<span class="sub small">{subtitle}</span>{/if}
        </span>
      {:else}
        <span class="meta">
          <span class="track">—</span>
          <span class="sub small">no track selected</span>
        </span>
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
        {#if playing}
          <svg width="11" height="11" viewBox="0 0 11 11" fill="currentColor" aria-hidden="true"><rect x="1" y="0.5" width="3.5" height="10" rx="0.5" /><rect x="6.5" y="0.5" width="3.5" height="10" rx="0.5" /></svg>
        {:else}
          <svg width="10" height="11" viewBox="0 0 10 11" fill="currentColor" aria-hidden="true"><polygon points="1,0.5 9.5,5.5 1,10.5" /></svg>
        {/if}
      </button>
      <button
        class="btn ctl"
        disabled={!canNext}
        onclick={onnext}
        title="next"
        aria-label="next"
      >
        <svg width="12" height="12" viewBox="0 0 12 12" fill="currentColor" aria-hidden="true"><polygon points="1,1 7,6 1,11" /><rect x="8.5" y="1" width="2.5" height="10" rx="0.5" /></svg>
      </button>
      <button
        class="btn btn-danger ctl"
        disabled={!active}
        onclick={onstop}
        title="stop"
        aria-label="stop"
      >
        <svg width="10" height="10" viewBox="0 0 10 10" fill="currentColor" aria-hidden="true"><rect x="1" y="1" width="8" height="8" rx="1" /></svg>
      </button>
    </div>
  </div>

  <!-- transport: elapsed · scrubber · total — its own full-width row below the band
       (area "bar"). Always present (disabled + 0:00 when idle) so the bar keeps an
       identical footprint across states. Sources with no known length (line-in,
       live streams) show elapsed only with a disabled track. -->
  <div class="transport">
    <span class="t-time small">{active ? position(dragging ? dragValue : displayPos) : position(0)}</span>
    <input
      class="t-bar"
      type="range"
      min="0"
      max={durationSec || 1}
      value={active ? (dragging ? dragValue : durationSec ? Math.min(displayPos, durationSec) : 0) : 0}
      step="0.1"
      disabled={!seekable}
      oninput={onSeekInput}
      onchange={onSeekCommit}
      title={seekable ? "seek" : "position"}
      aria-label="playback position"
    />
    <span class="t-time small">{active ? (durationSec ? position(durationSec) : "live") : position(0)}</span>
  </div>

  {#if active && queueLen > 0 && !expanded}
    <!-- collapsed: just the count on a non-selected card (queue list is noise there).
         Uses the gossiped length — no fetch needed when the card isn't selected. -->
    <div class="queue-collapsed small">{queueLen} in queue</div>
  {:else if active && queueLen > 0}
    <!-- expanded queue: the upcoming tracks under the now-playing bar, scrolling
         internally so ~10 are visible without growing the card unbounded. -->
    <div class="queue">
      <div class="queue-head small">Up next · {queueLen}</div>
      <ul class="queue-list">
        {#each queue as item, i (item.uri + ":" + i)}
          <li class="queue-item">
            <button
              class="q-play"
              onclick={() => onplay(i, item.uri)}
              title="play now"
              aria-label="play {queueTitle(item)} now"
            >
              <span class="q-idx small">{i + 1}</span>
              <span class="q-meta">
                <span class="q-title">{queueTitle(item)}</span>
                {#if queueSub(item)}<span class="q-sub small">{queueSub(item)}</span>{/if}
              </span>
            </button>
            <span class="spacer"></span>
            <button
              class="btn q-rm"
              onclick={() => onremove(i, item.uri)}
              title="remove from queue"
              aria-label="remove from queue"
            >
              <svg width="10" height="10" viewBox="0 0 10 10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" fill="none" aria-hidden="true"><line x1="2" y1="2" x2="8" y2="8" /><line x1="8" y1="2" x2="2" y2="8" /></svg>
            </button>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
</div>

<style>
  /* grid band: a fixed cover slot (col 1, spanning the band + scrubber rows) + the
     content column (col 2: now-playing band on top, full-width scrubber below); the
     queue spans full width under both. Identical footprint across idle / playing /
     queued (no reflow). Active tints accent; idle tints a faint muted so the playing
     room still stands out across the wall of cards. */
  .playbar {
    display: grid;
    grid-template-columns: 100px 1fr;
    grid-template-areas:
      "cov row"
      "cov bar"
      "que que";
    column-gap: 14px;
    row-gap: 6px;
    align-items: start;
    padding: 10px;
    border: 1px solid transparent;
    border-radius: 8px;
  }
  .playbar.active {
    background: color-mix(in srgb, var(--accent) 14%, transparent);
    border-color: color-mix(in srgb, var(--accent) 38%, transparent);
  }
  .playbar.idle {
    background: color-mix(in srgb, var(--muted) 7%, transparent);
    border-color: color-mix(in srgb, var(--muted) 22%, transparent);
  }

  /* cover slot (col 1): a fixed 100px square, top-aligned with the content. The
     sharp art is centered over a dimmed/blurred/cropped copy of itself. */
  .cover {
    grid-area: cov;
    width: 100px;
    height: 100px;
    align-self: start;
    position: relative;
    overflow: hidden;
    border-radius: 8px;
    background: var(--panel-2);
    border: 1px solid var(--border);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .cover .cover-bg {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    filter: blur(12px) brightness(0.22) saturate(1.05);
    transform: scale(1.15); /* over-scale so the blur never bleeds the edges in */
  }
  .cover .cover-art {
    position: relative; /* above the backdrop */
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: 6px;
  }
  /* reserved cover slot when there's no art: a CSS-only disc motif so the band
     keeps its exact footprint instead of collapsing. */
  .cover-placeholder {
    grid-area: cov;
    width: 100px;
    height: 100px;
    align-self: start;
    border-radius: 8px;
    background: var(--panel-2);
    border: 1px solid var(--border);
    display: grid;
    place-items: center;
  }
  /* a generic vinyl record, drawn with gradients: a black grooved disc with a
     diagonal specular sheen (::before), then a white center label with the
     spindle hole (::after). Both stack centered in the same grid cell. */
  .cover-placeholder::before {
    content: "";
    grid-area: 1 / 1;
    width: 84px;
    height: 84px;
    border-radius: 50%;
    background:
      /* specular sheen — two soft, opposed highlight arcs */
      conic-gradient(
        from 142deg,
        rgba(255, 255, 255, 0) 0deg,
        rgba(255, 255, 255, 0.22) 24deg,
        rgba(255, 255, 255, 0) 58deg,
        rgba(255, 255, 255, 0) 148deg,
        rgba(255, 255, 255, 0.15) 180deg,
        rgba(255, 255, 255, 0) 214deg,
        rgba(255, 255, 255, 0) 360deg
      ),
      /* fine grooves */
        repeating-radial-gradient(
          circle at 50% 50%,
          rgba(255, 255, 255, 0.05) 0 1px,
          rgba(0, 0, 0, 0) 1px 3px
        ),
      /* disc body, slightly lit toward the top */
        radial-gradient(circle at 50% 38%, #3a3a3a 0%, #141414 55%, #000 100%);
    box-shadow:
      0 2px 5px rgba(0, 0, 0, 0.55),
      inset 0 0 6px rgba(0, 0, 0, 0.6);
  }
  .cover-placeholder::after {
    content: "";
    grid-area: 1 / 1;
    width: 34px;
    height: 34px;
    border-radius: 50%;
    /* white label with a small spindle hole at the center */
    background: radial-gradient(
      circle at 50% 50%,
      #6f6f6f 0 1.6px,
      #f2f2f2 2.4px
    );
    box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.12);
  }

  /* the now-playing band (area "row"): state pill + track info + transport controls. */
  .pb-row {
    grid-area: row;
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }

  /* state pill — solid + colored so the state reads at a glance */
  .state-pill {
    flex: 0 0 auto;
    font-size: 10px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--panel-2);
    color: var(--muted);
  }
  .state-pill.playing {
    background: var(--ok);
    color: var(--ok-ink);
  }
  .state-pill.paused {
    background: var(--warn);
    color: var(--warn-ink);
  }

  /* center: icon + track (ellipsised so the controls never move) + position */
  .now {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .now .icon {
    flex: 0 0 auto;
    font-size: 15px;
  }
  /* title + subtitle stack; ellipsised so controls never move */
  .now .meta {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    justify-content: center;
    line-height: 1.2;
  }
  .now .track {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 14px;
  }
  .now .sub {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--muted);
  }
  /* idle: the em-dash placeholder track reads as muted, not as a real title. */
  .playbar.idle .now .track {
    color: var(--muted);
  }

  /* transport scrubber row (area "bar", full width below the band): elapsed · slider · total. */
  .transport {
    grid-area: bar;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 2px 0 0;
  }
  .t-time {
    flex: 0 0 auto;
    min-width: 3em;
    font-variant-numeric: tabular-nums;
    color: var(--muted);
    text-align: center;
  }
  .t-bar {
    flex: 1 1 auto;
    min-width: 0;
    height: 20px;
    accent-color: var(--accent);
    cursor: pointer;
  }
  .t-bar:disabled {
    opacity: 0.7;
    cursor: default;
  }
  /* idle: the reserved scrubber is faint + neutral, clearly inert. */
  .playbar.idle .t-bar {
    accent-color: var(--muted);
    opacity: 0.4;
  }

  /* right: square 36px icon buttons, identical footprint in every state */
  .controls {
    flex: 0 0 auto;
    display: flex;
    gap: 6px;
  }
  .controls .ctl {
    width: 36px;
    height: 36px;
    padding: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .controls .ctl:disabled {
    opacity: 0.4;
    cursor: default;
  }

  /* collapsed queue summary (non-selected card): just the count, unobtrusive. */
  .queue-collapsed {
    grid-area: que;
    color: var(--muted);
    padding: 0 2px 2px;
  }

  /* expanded queue under the bar: a bordered panel that scrolls internally so a
     long queue never grows the card; ~10 rows visible at a glance. */
  /* the expanded queue is the bottom slab of the green band: full-width, bled into
     the band's 10px padding, separated by an accent hairline rather than boxed. */
  .queue {
    grid-area: que;
    margin: 0 -10px -10px;
    border: none;
    border-top: 1px solid color-mix(in srgb, var(--accent) 24%, transparent);
    border-radius: 0 0 7px 7px;
    background: transparent;
    padding: 8px 10px 6px;
  }
  .queue-head {
    color: var(--muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    font-weight: 700;
    padding: 2px 2px 6px;
  }
  .queue-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    /* ~10 rows then scroll */
    max-height: 280px;
    overflow-y: auto;
    /* clear the scrollbar so the remove buttons never sit under it */
    padding-right: 12px;
  }
  .queue-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 3px 2px;
    border-radius: 6px;
  }
  .queue-item:hover {
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  /* the clickable body (index + media info) — plays that track now. A bare
     button reset so it reads as a row, with the gap/alignment the row used to own. */
  .q-play {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .q-play:hover .q-title {
    color: var(--accent);
  }
  .q-idx {
    flex: 0 0 auto;
    width: 1.8em;
    text-align: right;
    color: var(--muted);
    font-variant-numeric: tabular-nums;
    line-height: 1.2;
  }
  .q-meta {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    line-height: 1.2;
  }
  .q-title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 13px;
  }
  .q-sub {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--muted);
  }
  .q-rm {
    flex: 0 0 auto;
    width: 36px;
    height: 36px;
    padding: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .q-rm:hover {
    border-color: var(--danger);
    color: var(--danger);
    background: color-mix(in srgb, var(--danger) 10%, var(--panel-2));
  }

  /* Narrow viewports: drop the cover and reflow to four stacked rows —
     [pill · controls] / scrubber / track info / queue — and bleed the active
     (green) band to the card (viewport) edges. The .pb-row wrapper dissolves
     (display:contents) so its pill / info / controls place into the grid directly. */
  @media (max-width: 560px) {
    .playbar {
      grid-template-columns: 1fr auto;
      grid-template-areas:
        "pill ctl"
        "bar  bar"
        "inf  inf"
        "que  que";
      column-gap: 10px;
      row-gap: 8px;
    }
    .cover,
    .cover-placeholder {
      display: none;
    }
    .pb-row {
      display: contents;
    }
    .pb-row > .state-pill {
      grid-area: pill;
      align-self: center;
      justify-self: start;
    }
    .pb-row > .controls {
      grid-area: ctl;
      align-self: center;
    }
    .pb-row > .now {
      grid-area: inf;
      min-width: 0;
    }
    /* scrubber only — the time labels would crowd the narrow row */
    .t-time {
      display: none;
    }
    /* full-bleed the band (idle or active) + its queue to the card (viewport)
       edges. Pairs with app.css dropping #app's side padding and insetting cards
       by 14px. */
    .playbar {
      border-radius: 0;
      margin-inline: -14px;
      width: calc(100% + 28px);
      padding-inline: 14px;
    }
    .playbar .queue {
      margin-inline: -14px;
    }
  }
</style>
