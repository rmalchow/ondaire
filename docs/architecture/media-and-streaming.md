# Media sources & streaming

What a group's master *plays*, and how those frames reach every member. This covers
the media-source abstraction, the audio source server (the master-side fan-out), and
the subscription / prime / restart machinery on the receiving side.

## Media sources

What a master plays is a **URI**; the scheme selects an interchangeable media-source
implementation:

- **`file:<relative path>`** (or a bare path) — a file under the node's local media
  directory (`MEDIA_DIR`). `GET /api/media` lists playable files (recursive scan,
  extensions `.wav .mp3 .flac`). Paths are relative to `MEDIA_DIR`; traversal outside
  it is rejected.
- **`http://…` / `https://…`** — a remote stream or file (internet radio), decoded by
  Content-Type / URL extension. No duration, no seek.
- **`input:`** — the node's local capture input (line-in / mic), recorded via an
  exec-capture backend (`pw-record` / `arecord` pipe, mirroring the playback backends).

Any node's media can be browsed cluster-wide through the [node proxy](api.md).

### The source contract

All media sources implement one contract: produce canonical PCM (48 kHz / stereo /
s16le, 20 ms frames) via `ReadFrame(dst)` until `io.EOF`, plus `Close`. A scheme-keyed
factory (`file`, `http`, `input`) creates them; a node's available schemes are reported
in its capabilities. Two pacing classes:

- **Pull-paced** (`file`): decode runs ahead of the release ticker; `io.EOF` is the
  natural end of the session.
- **Live-paced** (`http`, `input`): data arrives in real time and never EOFs; the
  session ends only on `stop`. If the source momentarily underflows (network stall,
  capture hiccup), the release ticker emits **silence frames** — the stream's seq/pts
  cadence never stalls.

Adding a new kind of source (Spotify Connect, a snapcast pipe, …) means implementing
this one interface and registering a scheme.

## The audio source server (master side)

On `play`, the master starts an **audio source server** on its `SOURCE_PORT`:

1. It opens the media source for the URI and **releases canonical frames in real
   time** (a 20 ms ticker), each stamped `pts = sessionStart + frameIndex·20ms` in
   master-clock nanoseconds, where `sessionStart = now + leadMs`.
2. It keeps a **subscriber registry**: every group member — *including the master's
   own sink, which subscribes over loopback exactly like everyone else, with no special
   handling* — subscribes via the [stream control protocol](wire-protocol.md#stream-control).
   Each released frame is sent to every live subscriber. If the codec is opus, the
   master encodes once here, before fan-out.
3. It maintains a **ring buffer** of recently released frames, sized `max(2 ×
   bufferMs, 1 s)`, for priming newcomers (below).
4. It surfaces **source stats**: current subscriber count, total connects, restarts
   (re-prime requests), and primes served.

The registry keys subscribers by their observed `addr:port` — there is deliberately no
node-id → subscriber mapping in the fan-out hot path; node identity arrives on a
separate control path. (This is the constraint that shapes the unicast calibration
burst — see [`developer/calibration.md`](../developer/calibration.md).)

## Subscribing and priming

A member joins a stream by sending a **HELLO with the prime-me flag** to the master's
`SOURCE_PORT`, from the same socket it will read audio on (so over UDP the source learns
the audio return address by construction).

When a subscriber joins — or rejoins after getting lost — the source **burst-primes**
it: it replays every ring-buffer frame whose playout deadline (`pts + bufferMs`) is
still in the future. Older frames are useless to the newcomer and are skipped. The
burst is paced so it outruns the live stream without flooding:

- over **UDP**, ~4× realtime (one frame per ~5 ms);
- over **TCP**, back-to-back (flow control paces it).

After the burst, the subscriber receives live frames like everyone else. Priming is
what lets a mid-song join (or a recovery) land cleanly: the [sink](playout-pipeline.md)
gets a full buffer's worth of frames up front and primes its phase against the first
one whose deadline is still ahead.

A subscriber sends a **keepalive HELLO every 5 s**; the source expires any subscriber
unseen for **15 s**.

## Stop, end, and getting lost

- **`stop`** (or the natural EOF of a pull-paced source) bumps the generation,
  broadcasts a RECONFIG stop, and clears playback status.
- **Getting lost**: a subscriber whose frames cease for **> 2 s** sends a **RESTART**
  to the source — "I got lost, re-prime me" — and resumes from a fresh burst. If the
  source stays silent (the master died), the subscriber gives up, unsubscribes locally,
  and normal group [self-healing](discovery-and-cluster.md) takes over.

The full play / join / restart timelines are in [sequence diagrams](sequence-diagrams.md).
