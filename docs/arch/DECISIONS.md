# Contract reconciliation — integrator decisions

The eleven piece architects raised 43 contract concerns against
[S-skeleton.md](S-skeleton.md) and the [spec](../README.md). This file
resolves every one that needed a decision; trivially-confirmed items are
grouped at the end. **These decisions amend the arch docs** — where a piece
doc disagrees with this file, this file wins. (Surgical fixes already applied
to S-skeleton.md are marked ✎S.)

## Decisions

**D1 — node.json holds exactly `{id, name, volume, outputDelayMs, outputDevice}`**
*(amended by D35/D36/D37)*. (A) Everything else (following, ports, observations)
is runtime/replicated, in-memory only, rebuilt on start.

**D2 — `ENSEMBLE_OUTPUT` is env-only** (`auto` default | `null` | `file:<path>`
| explicit backend name). No flag. Added to spec §2. (A/E/K)

**D3 — capabilities are assembled by K (main) at startup** — PATH probe for
the exec tools **plus runtime dlopen probes** (D32: `libopus.so.0` →
`codecs:["pcm","opus"]`, `libasound.so.2` → `backends` includes `"alsa"`),
static format/scheme lists — and handed to cluster via its config/setter.
A stays pure, D stays decode-only. A node with `ENSEMBLE_OUTPUT=null` reports
`playback:false` but still receives and "plays" to the null sink; playback
capability never gates group membership or stream fan-out. (A/D/E/K)

**D4 — discovery `Peer` is `{ID id.ID; Addr netip.Addr; GossipPort, HTTPPort,
StreamPort int}`** with a `GossipAddrPort()` helper; B's channel is
`<-chan Peer`, closed on shutdown. zeroconf's SRV port carries HTTP_PORT but
is informational — **TXT records are authoritative** for all three ports. (B/C)

**D5 — group derivation is owned by C** (`cluster.DeriveGroups`, pure,
exported); `Snapshot.Groups` arrives pre-derived and joined with
names/playback/settings. H consumes `Snapshot.Groups` and does **not**
re-derive; H's own copy of the algorithm in H-group.md is dropped. (C/H)

**D6 — DialCandidates falls back to self-reported CIDRs** when the
observed-intersection is empty (cold peers must be dialable); it tightens to
observed-only as soon as any observation exists. Initial memberlist join uses
the discovery `Peer.Addr` directly — §3.1 resolution governs post-boot dials,
not cold bootstrap. Spec §3.1 wording adjusted. (C/K)

**D7 — own-record version reconciliation on restart**: after first push/pull,
if a peer holds our own record with version ≥ ours, jump our counter above it.
(C)

**D8 — gossip port handoff**: K uses netx to *probe* a free TCP+UDP pair for
the gossip port, closes both, and passes the bare number to memberlist (which
binds it itself). The tiny rebind race is accepted for v1. STREAM stays
bound-and-handed-over (mux keeps the UDP socket). (C/K)

**D9 — audio EOF semantics** (pinned for D and H): `ReadFrame(dst []byte)
error` fills exactly `stream.FrameBytes` into caller-owned `dst`; the final
partial frame is zero-padded and returned with `nil`; the *next* call returns
`io.EOF`. H's `Source` seam adopts this signature (H-group.md's
`ReadFrame() ([]byte, error)` is superseded). (D/H)

**D10 — `contracts.Clock` gains `LocalToMaster(localNanos int64) (int64, bool)`**
— H stamps PTS in master time and needs the forward conversion. ✎S (F/H)

**D11 — clock generation rides `Header.Gen`**; the 24-byte t1|t2|t3 payload
stands; the follower trusts its locally-recorded t1 keyed by `Header.Seq`
(echoed payload t1 is advisory). (F)

**D12 — no `Mux.Unregister` in v1.** Handlers are one-per-node and long-lived;
Receiver/Follower keep a `closed` guard so late dispatch is a no-op. (F/G)

**D13 — TCP stream framing is `uint32` big-endian length prefix** before each
`header+payload` chunk. Both ends live in G; pinned here so nobody invents a
second framing. FEC parity **is** flushed for a partial tail block on
stop/EOF. (G)

**D14 — cluster write-side method set** (concrete methods on `cluster.Cluster`,
not in `contracts`; H and I declare small consumer-side interfaces, Go-style):

```go
SetName(string)
SetVolume(float64)                                   // D35
SetOutputDelayMs(int)                                // D36
SetFollowing(id.ID)                                  // Zero = solo
SetPlayback(group id.ID, p contracts.Playback)
SetGroupSettings(group id.ID, s contracts.GroupSettings)
SetGroupName(group id.ID, name string)
Observe(peer id.ID, ip string)
DialCandidates(peer id.ID) []netip.Addr              // best-first
Join(addrs []string) error                           // seed list / discovery
```
(C/H/I)

**D15 — go:embed lives in `web/embed.go`** (`package web`,
`//go:embed all:dist`, exports `DistFS`), because `go:embed` cannot reference
parent dirs from `internal/api`. The API piece takes the FS via its config.
✎S (I)

**D16 — `FollowClient` is implemented in `internal/api` as a plain
cluster-backed HTTP client** (no dependency on the Echo server), so K builds:
cluster → followClient → group engine → api server. No construction cycle.
(H/I/K)

**D17 — takeover forwarding is I's job** (proxy hop to current master);
`group.MakeMaster` assumes it executes on the master and errors with
`ErrNotMaster` otherwise. H owns re-pointing the clock follower
(`SetMaster(addr, gen)`) whenever the elected master endpoint or generation
changes. *(Endpoint-management half superseded by D22 — the subscribe model
removes per-member stream endpoints; clock re-pointing stands.)* (H/K)

**D18 — ~~stream endpoints~~ SUPERSEDED by D22**: there is no `Resolver` /
`SetEndpoints` seam. Subscribers dial the master's `SOURCE_PORT` (resolved
via `cluster.DialCandidates(master)`); the source streams back to the address
each subscription actually came from. (H/K)

**D19 — `/api/status` JSON envelope** (pinned for I, J, and the e2e):

```json
{
  "id": "<32hex>", "name": "...", "role": "master|follower|solo",
  "groupId": "<32hex>",
  "ports": {"http": 8080, "stream": 9090, "source": 9200, "gossip": 7946},
  "sink":  {"played": 0, "silence": 0, "lateDrop": 0, "staleGen": 0,
            "synced": false, "ratePPM": 0, "buffered": 0},
  "clock": {"synced": false, "offsetNs": 0, "rttNs": 0},
  "source": {"clients": 0, "connects": 0, "restarts": 0, "primes": 0}
}
```
(I/K; `role:"solo"` = master of a group of 1; `source` present only while
this node runs an active audio source.)

**D20 — `--join` / `ENSEMBLE_JOIN`** (comma-separated `host:gossipPort` seed
list) is added as a dev flag in A, passed to `cluster.Join`. It exists for
hermetic loopback e2e tests; mDNS remains the production path. Added to spec
§2 as dev-only. (K)

**D21 — ~~bufferMs is fixed per session~~ PARTIALLY SUPERSEDED by D23**:
settings changes now apply live — the master bumps the generation and
broadcasts RECONFIG; subscribers re-fetch replicated group settings and
resubscribe (spec §8.7). Still true: sink `Stats().Synced` is computed live
from the Clock at call time; `Backend.Write` may block; the exec backend gets
a write deadline via process kill on Close — accepted v1 limitation. (E)

---

## Audio source/sink restructure (second review round)

Spec §6/§8 were reworked after user review: subscribe-based streaming on a
dedicated SOURCE_PORT, source ring + burst priming, live settings changes,
a continuous DAC rate servo, and interchangeable source/backend
implementations. Decisions D22+ pin the parameters; arch docs D, E, G, H, K
were regenerated against them.

**D22 — subscribe model on SOURCE_PORT (default 9200, TCP+UDP,
bind-or-increment)**: the source listens; members subscribe via stream
control (§8.7: HELLO/BYE/RESTART/RECONFIG, packet types 0x20–0x23, 1-byte
flag payload). UDP subscribers HELLO **from their STREAM_PORT mux socket**,
so audio flows back to the observed source addr:port and the member-side
receive path (mux types 0x01/0x02) is unchanged. TCP subscribers dial
SOURCE_PORT; control + length-prefixed audio share the connection. HELLO
keepalive every 5 s; subscriber expiry 15 s. The master's own sink subscribes
over loopback like any client. Inbound SOURCE_PORT only matters on masters,
but every node binds it (any node can become master). (G/H/K)

**D23 — live settings changes**: master bumps gen, broadcasts RECONFIG,
refreshes the replicated group-settings record; subscribers re-read settings
and resubscribe under the new gen. RECONFIG with the stop flag is the
explicit end-of-session notice (§8.6). (G/H)

**D24 — source ring & burst prime**: ring of released frames sized
`max(2 × bufferMs, 1 s)`. Prime = replay ring frames whose `pts + bufferMs`
deadline is still future (older frames are skipped — useless to the
newcomer). UDP burst pacing ~4× realtime (one frame per ~5 ms); TCP
back-to-back. Primes are counted in SourceStats (at burst initiation).
*Implementation refinement:* a priming subscriber is **excluded from live
fan-out** until its burst has caught up to the live edge via the ring
(`framesAfter` loop) — otherwise an interleaved live frame would anchor the
newcomer's reorder window/sink ahead of the burst and the entire prime would
be dropped as late. The >‑realtime burst rate guarantees catch-up
terminates. (G)

**D25 — rate servo (E)**: skew estimator — cumulative samples consumed vs
master-clock elapsed, ~3 s averaging window; backend `DelayReporter`
(`DeviceDelay()`) used when implemented, backpressure inference otherwise —
feeding a PI controller, output clamped ±500 ppm and slew-limited, driving a
4-tap Catmull-Rom fractional resampler between jitter buffer and backend.
Runs continuously (drift *prevention*); underruns stay silence + watchdog →
RESTART (§8.6: starved > 2 s → RESTART to the source; still starved →
unsubscribe, group self-heal takes over). Target buffer level = bufferMs.
`SinkStats` gains `RatePPM`, `Buffered`. (E/G)

**D26 — media-source abstraction (D)**: scheme-keyed factory `file` / `http` /
`input` → one `Source` contract (canonical-PCM `ReadFrame(dst)`, `Close`,
D9 EOF semantics). Pull-paced (`file`: decode-ahead, EOF ends session) vs
live-paced (`http`/`input`: never EOF, underflow → the release ticker emits
silence, cadence never stalls). `input` is exec-capture (`pw-record`/
`arecord`), mirroring the exec playback backend. Available schemes reported
in `capabilities.sources`. (D/H)

**D27 — sink-backend registry (E)** *(amended by D32 — no build tags)*:
named backends `alsa` (runtime-loaded libasound, implements `DelayReporter`,
**v1**), `exec` (fallback pipe), `null`, `file` — all in the one and only
build; `alsa` registers itself only when the dlopen probe succeeds.
`ENSEMBLE_OUTPUT` selects by name; `auto` picks **alsa → exec → null**.
Available names reported in `capabilities.backends`; `playback` = a real
(non-null) backend is usable. (E/K)

**D28 — source stats surfacing**: `SourceStats{Clients, Connects, Restarts,
Primes}` — in `/api/status` (D19) and riding the master-written replicated
playback record (`Playback.Source`), refreshed with the periodic position
update, so the UI reads it from the cluster snapshot. (G/C/I/J)

**D29 — seam names follow G's concrete exports** (consistency-sweep
resolution): the source server is `source.NewServer(source.Config)` with
`StartSession / ReleaseFrame / Reconfig / StopSession / Stats`; the
subscriber is `stream.NewClient` (package `internal/stream`) with
`Subscribe(sourceAddr, gen, transport)` / `Unsubscribe` / `Counters`. H's
consumer-side interfaces adopt these method names (its `Publish` /
`SubscribeTo` / `EndSession` spellings in H-group.md are superseded). (G/H/K)

**D30 — live-source underflow is D's problem, not H's**: live sources
(`http`, `input`) emit silence internally on momentary underflow and
`ReadFrame` returns `nil` — there is **no** `audio.ErrUnderflow` sentinel;
H-group.md's references to it are superseded. The release ticker always gets
a frame. D's `Open(ctx, uri, mediaDir)` signature stands; H bridges with a
closure. (D/H)

**D31 — no `api.SetGroup`**: H depends on I only via `contracts.FollowClient`
(leaf package, no cycle), so K builds the API server **last** with the group
engine in `api.Config` and obtains the follow client via
`apiSrv.FollowClient()` — actually constructed standalone before the engine:
`api.NewFollowClient(cluster)` → `group.New(...)` → `api.New(Config{Group:
engine, ...})`. K-main-e2e.md's `SetGroup` fallback is unused. (H/I/K)

---

## Runtime library loading (third review round)

User question: can the binary ALWAYS carry libopus/libalsa support and probe
loadability at runtime, degrading gracefully? Yes — and it kills the
build-tag convention entirely. There is exactly **one build**, no cgo.

**D32 — runtime loading via purego (`internal/dl`, S-owned)**: optional
shared libraries are loaded with `github.com/ebitengine/purego`
(dlopen/dlsym FFI from pure Go, works with CGO_ENABLED=0).
`dl.Open(sonames, symbols)` tries sonames in order (`libopus.so.0` then
`libopus.so`; `libasound.so.2` then `libasound.so`), and **dlsym-verifies
every required symbol before any `RegisterLibFunc`** — a missing library,
wrong version, or missing symbol yields `dl.ErrUnavailable` (soft), never a
panic, and the corresponding capability is simply reported off (D3).
Call-rate is ~50/s (per 20 ms frame) so FFI overhead is irrelevant. The
CGO_ENABLED=0 static-build interaction is purego's documented supported mode;
verified at implementation time. Build tags for `opus`/`alsa` are abolished
everywhere. (S/D/E/K)

**D33 — opus placement & validation (supersedes the §8.3 build-tag text)**:
the codec module lives in `internal/audio` (piece D):
`audio.NewOpusEncoder() / NewOpusDecoder()` return `dl.ErrUnavailable` when
libopus isn't loadable. The ~7 functions bound: `opus_encoder_create`,
`opus_encoder_ctl` (bitrate 128k), `opus_encode`, `opus_encoder_destroy`,
`opus_decoder_create`, `opus_decode`, `opus_decoder_destroy`. **Master
encodes once** (H wires it between source `ReadFrame` and `source.Server`
fan-out); **each member decodes** (wired between the subscriber's deliver
callback and `Sink.Push` — the sink always consumes canonical PCM). Before
starting an opus session the master checks that every current member reports
the `opus` capability and rejects `play` naming the nodes that lack it.
No packet-loss concealment on the decoder — a lost opus frame is silence,
same as pcm (§8.5); keep simple. (D/E/G/H)

**D34 — alsa backend (E, v1)**: simple-API binding via `internal/dl`:
`snd_pcm_open(&pcm, "default", PLAYBACK, 0)`,
`snd_pcm_set_params(pcm, S16_LE, RW_INTERLEAVED, 2, 48000, 1, latencyUs)`,
`snd_pcm_writei` per frame with `snd_pcm_recover` on `-EPIPE`/`-ESTRPIPE`,
`snd_pcm_delay` implementing `contracts.DelayReporter` (exact servo
measurement), `snd_pcm_close`. Registers in the backend registry only when
the probe succeeds; first in `auto` order (D27). (E)

---

## Per-node volume & output-delay calibration (user addition)

**D35 — per-node volume (live software gain)**: `volume` float `0.0–1.0`,
default `1.0`. Stored in `node.json` (D1 amended: `{id, name, volume,
outputDelayMs}`) and the replicated node record; set via
`PATCH /api/node {volume}` (UI proxies to the node). Applied in the sink as
the last stage before the backend (`Sink.SetGain`): per-sample int16
multiply, target read atomically each frame, linear ramp over one 20 ms
frame on change — continuous, no restart. Gain applies on every backend
(incl. null/file). `0.0` is a real value (muted): absent-field defaulting to
`1.0` happens **only** in A's presence-aware node.json decode; every layer
downstream (K→C→E) treats the resolved value as authoritative — no
zero-means-unset remapping anywhere. Hardware-mixer volume is out of scope
v1. (A/C/E/I/J)

**D36 — per-node output-delay calibration**: `outputDelayMs` int, default 0,
clamped ±500. Compensates fixed downstream latency invisible to both the
servo and `DeviceDelay()` (pipe player internals, DAC/amp/BT chains).
Playout deadline = `MasterToLocal(pts) + bufferMs − outputDelayMs`. Stored
like D35; set via `PATCH /api/node {outputDelayMs}`. Sign convention:
**positive = the device chain is late → write earlier** (the deadline
subtracts it); `Sink.SetDelayOffset` takes nanoseconds (I converts
`ms · 1e6`). A live change calls `Sink.SetDelayOffset` → the sink drops its
buffer and fires the restart hook (RESTART → burst re-prime, §8.6) — the
user-visible cost is a sub-second blip on that node only. (A/C/E/I/J)

## Output-device selection (user addition)

**D37 — output-device selection**: each node may select which ALSA device the
alsa backend opens, instead of always `default`. *Enumeration source* is
`/proc/asound/pcm`, parsed with zero external deps (`sink.ListOutputDevices` →
`parseProcPCM`, pure/testable): playback-capable `CC-DD` lines become
`{ID:"hw:C,D", Desc:<id>}`, prepended with `{ID:"default", Desc:"system
default"}`. The list is empty when libasound is not loadable (the alsa backend
never registered → selection is meaningless) OR `/proc/asound/pcm` is missing.
It is enumerated **once at startup** and reported on the node record
(`OutputDevices []contracts.OutputDevice`) plus the resolved `NodeView`.
*Persistence*: `node.json` gains `outputDevice` (default `"default"`,
presence-aware decode + clamp/normalize like `volume`); `config.SetOutputDevice`
mirrors `SetVolume`. *Selection semantics*: `sink.OpenDevice(spec, device, log)`
routes the configured device down the alsa path (auto-selected or explicit
`alsa`); every other backend ignores it — the **exec backend ignores the device
in v1** (plays to its tool's own default). *Live apply*: `PATCH /api/node
{outputDevice}` validates against the node's own enumerated list or `"default"`
(≤64 chars, non-empty), then persist (A) → replicate (C, `SetOutputDevice`) →
apply: only when the active backend kind is alsa, K reopens the backend for the
new device and calls `Playout.SwapBackend(b)` (under the sink mutex: close old,
set new, re-assert `DelayReporter`, log `output backend swapped`). A brief audio
blip is accepted; the session is **not** restarted. (A/C/E/I/J/K)

## Confirmed as designed (no change)

- C's two-mutex exception (doc + liveness) with a never-hold-both rule. (C)
- D imports only the PCM constants from `package stream`. (D)
- Sink `Push` is fire-and-forget; no backpressure/close signal to G. (E/G)
- G's transport `Counters` and E's `SinkStats` stay separate; `/api/status`
  surfaces sink stats (D19); transport counters may be added later. (G/I)
- `/api/status` carries only `groupId`/`role`; the full group object comes
  from `/api/cluster`. (I)
- Loopback e2e: nodes on 127.0.0.1 have empty `InterfaceCIDRs`; reachability
  comes from `--join` seeds + observed-IP reporting (memberlist + HTTP traffic
  both feed `Observe`). (K)
- K reconciles exact constructor names at integration (the fix-loop). (K)
