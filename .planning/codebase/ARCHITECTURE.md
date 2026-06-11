<!-- refreshed: 2026-06-11 -->
# Architecture

**Analysis Date:** 2026-06-11

## System Overview

```text
┌────────────────────────────────────────────────────────────┐
│              REST API + WebSocket (I)                       │
│         `internal/api` — Echo server, routes, WS            │
├────────────────────────────────────────────────────────────┤
│  Group Engine (H)   │ Playback Control (D59) │ Spotify (D57)│
│  `internal/group`   │ `internal/playback`    │ `internal/spotify`
├─────────────┬───────┴──────────┬──────────────┴──────┬──────┤
│   Source    │  Subscriber      │  Sink (Playout)    │ Clock │
│   Server    │  Stream Client   │  `internal/sink`   │ Server│
│  `internal/ │ `internal/stream`│                    │ (F)   │
│   source`   │                  │                    │       │
├────────────────────────────────────────────────────────────┤
│         Cluster State & Gossip (C)                          │
│  `internal/cluster` — memberlist, replicated LWW doc        │
├─────────────┬──────────────────────────────────────────────┤
│ Discovery   │  Network Binding                              │
│ (B)         │  `internal/netx` — port bind/interface scan   │
│ mDNS        │                                                │
│ `internal/  │  UDP Mux (S)                                  │
│ discovery`  │  `internal/stream` — wire codec, packet mux   │
├────────────────────────────────────────────────────────────┤
│  Config (A)  │  Runtime DL Probe (S)  │  ID Contract (S)   │
│  `internal/` │  `internal/dl` — dlopen│  `internal/id`     │
│  config`     │  libopus, libasound    │  16-byte ID, XOR   │
└────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| **Config (A)** | Load flags/env, node.json minting, directory resolution | `internal/config/config.go`, `internal/config/node.go` |
| **Discovery (B)** | mDNS register + browse, peer enumeration | `internal/discovery/discovery.go` |
| **Cluster (C)** | Gossip state replication, LWW docs, member tracking, liveness | `internal/cluster/cluster.go`, `internal/cluster/doc.go` |
| **Audio Sources (D)** | Scheme-keyed media factory, decoders, opus/PCM encode, line-in capture | `internal/audio/source.go`, `internal/audio/opus.go` |
| **Sink/Playout (E)** | Jitter buffer, rate servo, resampler, gain, backend registry | `internal/sink/sink.go`, `internal/sink/servo.go` |
| **Clock (F)** | Master/follower NTP-like sync on UDP; pts/deadline translation | `internal/clock/clock.go`, `internal/clock/sample.go` |
| **Stream & Source (G)** | Wire frame codec, UDP mux, source fan-out, subscriber TCP+UDP | `internal/stream/wire.go`, `internal/source/server.go` |
| **Group Engine (H)** | Play/pause/seek, group membership, takeover, source/sink binding | `internal/group/engine.go`, `internal/group/play.go` |
| **HTTP API (I)** | REST routes, WebSocket, node proxy, SPA serving | `internal/api/api.go`, `internal/api/handlers.go` |
| **Web UI (J)** | Svelte 5 SPA, WS store, fetch wrappers, media browser | `web/src/App.svelte`, `web/src/lib/ws.svelte.js` |
| **Main & E2E (K)** | Component wiring, port binding, capability probing, graceful shutdown | `cmd/ensemble/main.go` |

## Pattern Overview

**Overall:** Layered dependency-injection architecture with a single-pass bottom-up build, LIFO shutdown, and contract-driven interfaces to avoid cycles.

**Key Characteristics:**
- Each component owns one disjoint set of files; cross-piece contracts in `internal/contracts/contracts.go`
- Initialization order: A → B/C/D/E → F/G → H → I; each depends on layers below
- Gossip-driven state replication (memberlist + LWW) eliminates a central database
- Media sources and output backends are pluggable registries (not hard-coded chains)
- Clock follower provides wall-time → master-time translation for synchronized playout
- No cgo, no build flags; optional runtime libraries (Opus, ALSA) probed at startup via `internal/dl`

## Layers

**Layer 1 — Contracts & Foundation (S):**
- Purpose: Define cross-piece interfaces and shared wire format
- Location: `internal/contracts/`, `internal/stream/wire.go`, `internal/stream/mux.go`, `internal/id/`, `internal/netx/`, `internal/dl/`
- Contains: Interface types (Backend, Sink, Clock, StateStore), frame wire codec, UDP mux, ID type, port binding, dynamic library loading
- Depends on: Standard library only
- Used by: Every component

**Layer 2a — Configuration & Discovery (A, B):**
- Purpose: Load node identity and enumerate peers
- Location: `internal/config/`, `internal/discovery/`
- Contains: Flag/env parsing, node.json persistence, name/ID/volume/delay calibration, mDNS registration and browsing
- Depends on: S (contracts, netx, id)
- Used by: Cluster (C), main (K)

**Layer 2b — Media & Audio (D, E):**
- Purpose: Decode audio files, apply codecs, handle output backends
- Location: `internal/audio/`, `internal/sink/`
- Contains: File decoders (WAV/MP3/FLAC), HTTP live streaming, line-in capture, opus encode/decode, ALSA/exec/null/file output backends, jitter buffer, rate servo, resampler, gain stage
- Depends on: S (contracts, id, dl)
- Used by: Group engine (H), main (K)

**Layer 2c — Cluster & Gossip (C):**
- Purpose: Replicate node state across the network
- Location: `internal/cluster/`
- Contains: memberlist wrapper, LWW document store, node records, group names, playback status, liveness tracking, change subscriptions
- Depends on: S (contracts, id), B (discovery peers)
- Used by: Group engine (H), API (I)

**Layer 3 — Timing & Transport (F, G):**
- Purpose: Synchronize clocks and stream audio frames
- Location: `internal/clock/`, `internal/stream/`, `internal/source/`
- Contains: Clock server (UDP 0x10/0x11), clock follower with offset tracking, UDP mux dispatcher, source server (fan-out, FEC, TCP keepalive), subscriber client (reorder window, FEC recovery)
- Depends on: S (contracts, stream/wire, mux), C (state for peer candidates)
- Used by: Group engine (H), main (K)

**Layer 4 — Group Logic & Playback (H):**
- Purpose: Orchestrate group membership, media playback, and sink subscription
- Location: `internal/group/`, `internal/playback/`, `internal/spotify/`
- Contains: Play/pause/seek state machine, follow/unfollow, group takeover, media source binding, session generation, reconfig broadcast, playback node control driver, Spotify Connect bridge manager
- Depends on: All lower layers (C for state, D for media, E for sink, F for clock, G for stream)
- Used by: API (I), main (K)

**Layer 5 — HTTP Interface & UI (I, J):**
- Purpose: REST API, WebSocket updates, SPA serving
- Location: `internal/api/`, `web/`
- Contains: Echo server routes, REST handlers, WebSocket debounce + heartbeat, node proxy, SPA with media browser, group/node controls, settings UI
- Depends on: All lower layers via contracts and adapter interfaces
- Used by: Browser clients, other nodes (proxy)

**Layer 6 — Wiring & Lifecycle (K):**
- Purpose: Build component graph, bind ports, probe capabilities, manage startup/shutdown
- Location: `cmd/ensemble/main.go`
- Contains: parseOptions, run(), port binding, capability probing, LIFO shutdown stack, graceful signal handling
- Depends on: All components
- Used by: Binary entry point

## Data Flow

### Primary Request Path — Group Play

1. **User calls POST /groups/{id}/play** (`internal/api/handlers.go:217`)
   - API handler validates URI, codec, transport
2. **Handler calls H.Play(ctx, uri, codec, transport)** (`internal/group/play.go:79`)
   - Resolves media source via scheme-keyed factory (D) → file/http/input/spotify
   - Opens media source, starts ticker for frame releases
3. **H starts source server** (`internal/source/server.go:86`)
   - Broadcasts RECONFIG (gen++, settings) to all subscribers
   - Builds ring buffer of released frames
4. **H opens subscriber to its own source** (in-process loopback)
   - Client receives HELLO, decodes frames (opus if configured), pushes to sink
5. **Subscriber decode callback** (`internal/stream/client.go:89`)
   - Reads from source UDP/TCP, fills jitter buffer via Sink.Push(gen, seq, pts, PCM)
6. **Sink scheduler loop** (`internal/sink/sink.go:177`)
   - Waits for frame with pts ≤ now (wall-time + buffer)
   - Applies rate servo (skew correction), resampling, gain, writes to backend
   - Clock.MasterToLocal(pts) translates source timestamp to local deadline

### Remote Member Join

1. **Member calls POST /groups/{id}/join** via API → H.Follow(master_id)
2. **H merges groups** (XOR of member IDs becomes the group ID)
3. **H sets clock follower master** (C.DialCandidates for address resolution)
4. **Clock follower** sends 0x10 requests at 1 Hz, receives 0x11 replies, computes 5-best-of-30 median offset
5. **H opens subscriber to master's source**
   - Subscriber sends HELLO to master's SOURCE_PORT
   - Master registers subscriber, begins burst prime (release ring buffer)
6. **Subscriber receives frames**, sink plays them via the same path as the master

### Sink Playout

1. **Sink Reset(gen)** arms for new session, zeros counters
2. **Push(gen, seq, pts, PCM)** enqueues frame in jitter buffer (non-blocking, drops late/stale)
3. **Scheduler (10 ms poll loop)**
   - Checks if head of jitter buffer is due (pts ≤ deadline)
   - Computes deadline = clock.MasterToLocal(pts) − outputDelayMs − deviceLatencyMs
   - Applies rate servo correction (continuous ±500 ppm skew feedback)
   - Resamples if needed (Catmull-Rom 4-tap)
   - Applies live gain (atomic, one-frame ramp)
   - Writes frame to backend (alsa/exec/null/file)
4. **Watchdog** (2 s starvation timeout)
   - Fires RESTART hook if sink starves (injects subscriber.Restart())

### State Synchronization (Gossip)

1. **Cluster.SetName(name)** atomically updates this node's record in the LWW doc
   - Bumps version, broadcasts via memberlist push/pull TCP (delegate)
2. **Peers exchange full snapshots** (gossip-pull every ~5s) + incremental updates (push on change)
3. **Converges in O(log N) rounds** for uniform cluster state
4. **API/WebSocket broadcast changes** via Subscribe channel coalesced over 50 ms

**State Management:**
- All state is in-memory LWW documents (no persistent cluster store; restart reads cluster.json if it exists)
- Playback state (now-playing URI, position, codec, transport) is only on the group master
- Group membership is computed server-side from following links: `members = {m | m is alive AND (m == master OR m.Following == master)}`
- Group ID = master's ID (takes the master's ID when a member takes over)

## Key Abstractions

**Media Source Factory (scheme-keyed registry):**
- Purpose: Open a source given a URI (`file://`, `http(s)://`, `input://`, `spotify:`)
- Examples: `internal/audio/source.go:91` (Open), `internal/audio/file.go`, `internal/audio/http.go`, `internal/audio/input.go`
- Pattern: Each source implements `ReadFrame(dst []byte) error` filling 3840-byte canonical PCM frames

**Audio Codec Module (opus):**
- Purpose: Transcode between PCM (3840 B/frame) and Opus (≈320 B/frame at 128 kbps)
- Example: `internal/audio/opus.go:24` (Encoder/Decoder)
- Pattern: Runtime-probed via `internal/dl`; graceful degradation when libopus unavailable

**Output Backend Registry (named plugins):**
- Purpose: Play PCM to ALSA/exec/null/file; switch between backends on failure
- Examples: `internal/sink/backend_alsa.go`, `internal/sink/backend_exec.go`, `internal/sink/backend_resilient.go:29`
- Pattern: Each backend implements `contracts.Backend` interface; resilient wrapper tries next on error

**Subscriber (UDP + TCP multiplexed receiver):**
- Purpose: Subscribe to a source, receive frames over UDP (fast-path) + TCP (reliability), decode opus, hand to sink
- Example: `internal/stream/client.go:61` (Client.subscribe)
- Pattern: FEC recovery window reorders packets, recovers lost frames via XOR parity

**Cluster State Store (LWW doc):**
- Purpose: Single source of truth for node records, group names, playback status
- Example: `internal/cluster/store.go:45` (snapshot construction)
- Pattern: Immutable snapshots, change notification channel, setter methods bump version + broadcast

## Entry Points

**`cmd/ensemble/main.go` — Binary startup:**
- Location: `cmd/ensemble/main.go:56` (main)
- Triggers: Process start (or `ensemble run …` alias)
- Responsibilities:
  1. Parse flags/env via parseOptions
  2. Load config (A) → mints ID, resolves DataDir/MediaDir
  3. Probe capabilities: libasound, libopus, PATH scan
  4. Bind four ports (HTTP, Stream, Source, Gossip)
  5. Build component graph bottom-up (S→A→B/C→F/G→E→H→I)
  6. Run mux, cluster, clock, sink, source, engine, Spotify, playback driver, API
  7. Block on signal, teardown LIFO on SIGINT/SIGTERM

**`cmd/ensemble/main.go:490` — Group engine run loop:**
- Called: Engine.Run(ctx) goroutine at startup
- Responsibilities:
  - Watch cluster for my group changes
  - Validate group membership (liveness, self-heal)
  - Orchestrate takeover (if I become master)
  - Drive playback state machine (play/pause/seek)
  - Manage session generation + reconfig broadcasts
  - Subscribe/unsubscribe to active source
  - Inject restart hooks into sink

**`internal/api/handlers.go` — HTTP request entry points:**
- Location: `internal/api/handlers.go`
- Responsibilities:
  - POST /groups/:id/play → group.Play(uri)
  - POST /groups/:id/pause → group.Pause()
  - POST /nodes/:id/follow/:target → group.Follow(target)
  - POST /nodes/:id/name → cluster.SetName(name)
  - GET /cluster → cluster.Snapshot()
  - WebSocket /ws → debounced cluster + 5 s heartbeat pushes

**`internal/source/server.go:86` — Source server main loop:**
- Called: sourceServer.Run() at startup
- Responsibilities:
  - Listen on SOURCE_PORT (TCP+UDP)
  - Accept HELLO from subscribers
  - Maintain subscriber registry (address, keepalive, generation)
  - Fan-out released frames + FEC parity
  - Respond to RESTART (re-prime from ring buffer)
  - Send RECONFIG on settings change (generation/codec/transport)

**`internal/sink/sink.go:177` — Playout scheduler loop:**
- Called: Playout.scheduler() goroutine
- Responsibilities:
  - 10 ms poll loop over jitter buffer
  - Drop stale/late frames, count them
  - Translate pts via clock.MasterToLocal()
  - Apply rate servo correction + resampling
  - Apply live gain stage
  - Write canonical frames to backend
  - Starvation watchdog (RESTART if starved > 2s)

## Architectural Constraints

- **Threading:** Single-threaded per component; sync via channels (mux broadcasts), mutexes (cluster doc, sink), or atomics (gain, delay offset). The main scheduler goroutine in sink is the hot path.
- **Global state:** None. Every component is a struct passed as a parameter; cluster is Singleton-ish (K owns one instance, H+I reference it).
- **Circular imports:** None. Contracts are in a separate `internal/contracts/` leaf package; each layer depends downward only.
- **Clock consistency:** Master clock is monotonic-ns (not wall-clock); follower computes offset to translate at playout time. Multi-room sync relies on synchronized pts (from master's mono clock) + offset convergence.
- **Port binding:** Bind-or-increment for unset ports; pinned (flag/env) ports fail hard if unavailable (early error detection).
- **Capability discovery:** Runtime probed at startup, static defaults (wav/mp3/flac, null/file backends always available). Once probed, capabilities are frozen for the node's lifetime.

## Anti-Patterns

### Panic on unrecoverable errors

**What happens:** Some early-startup failures (bad port, corrupt node.json) call `os.Exit(1)` in main rather than propagating errors gracefully.

**Why it's wrong:** The LIFO shutdown stack isn't unwound; listeners/sockets leak (though OS cleans them up on exit). Tests can't inject failures.

**Do this instead:** Always return errors from run(ctx) and let main handle cleanup. For startup probes (capability detection), always gracefully degrade rather than fataling.

### Unbounded goroutines in stream handlers

**What happens:** Each incoming subscriber creates a decode callback goroutine that's never explicitly stopped.

**Why it's wrong:** If many subscribers connect/disconnect, goroutine leak. Resource exhaustion under sustained churn.

**Do this instead:** Track active subscribers in a map, drain them on Close(), wait for goroutines in the shutdown path.

## Error Handling

**Strategy:** Named sentinel errors + context wrapping. Typed errors (ErrNoOpus, ErrNotSynced, etc.) surface to API as HTTP status codes; context wrapping (fmt.Errorf) chains causes.

**Patterns:**
- `contracts.Sink.Push()` is non-blocking: late/stale frames are dropped and counters incremented (no error returned)
- Stream subscription errors (bad codec, no transport) return quickly with a typed error (no long-lived goroutine)
- Clock synchronization waits indefinitely for the first sample; playout gates on `Synced() bool` (doesn't wait)

## Cross-Cutting Concerns

**Logging:** `log/slog`, component-scoped via `With("comp", "cluster")`. Main wraps all errors with `fmt.Errorf` to add context. Structured (key-value) for grep-ability.

**Validation:** Per-route in API handlers; media sources validate URI format (scheme, path existence). Cluster validates group membership (liveness, cycles). Sink validates frame header (magic, type, sequence).

**Authentication:** None. v1 is trusted-LAN only (no TLS, no auth tokens).

---

*Architecture analysis: 2026-06-11*
