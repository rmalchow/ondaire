# Codebase Structure

**Analysis Date:** 2026-06-11

## Directory Layout

```
ensemble/
├── cmd/                    # Binaries
│   ├── ensemble/          # Main multiroom audio daemon
│   ├── player/        # Wire protocol reference client (ESP32)
│   └── soundcheck/        # Audio device diagnostic tool
├── internal/              # All runtime logic (not importable externally)
│   ├── contracts/         # Skeleton: cross-piece interfaces (Backend, Sink, Clock, StateStore)
│   ├── id/                # Node ID type + XOR operation (16-byte deterministic)
│   ├── netx/              # Network binding: BindTCP/UDP, InterfaceCIDRs, CIDR utilities
│   ├── dl/                # Dynamic library loading: dlopen (libopus, libasound) with ErrUnavailable
│   ├── stream/            # Wire codec, UDP mux, subscriber (source/sink transport)
│   ├── config/            # Flags, env, node.json persistence, port resolution
│   ├── discovery/         # mDNS register + browse (zeroconf)
│   ├── cluster/           # Gossip state replication (memberlist + LWW doc)
│   ├── audio/             # Media sources: decoders (wav/mp3/flac), http, input, opus codec
│   ├── sink/              # Playout: jitter buffer, rate servo, backends (alsa/exec/null/file)
│   ├── clock/             # Master/follower clock sync (UDP 0x10/0x11)
│   ├── source/            # Source server: fan-out, FEC, TCP keepalive
│   ├── group/             # Group engine: play/pause/seek, membership, takeover
│   ├── playback/          # Playback node control driver (master→remote commands)
│   ├── api/               # REST handlers, WebSocket, SPA serving, node proxy
│   └── spotify/           # Spotify Connect bridge (go-librespot)
├── web/                   # Web UI (Svelte 5)
│   ├── src/
│   │   ├── App.svelte     # Layout, page routing
│   │   ├── main.js        # Vite entry
│   │   ├── app.css        # Global styles
│   │   ├── components/    # Reusable UI components
│   │   ├── sections/      # Page sections: Groups, Nodes
│   │   ├── lib/           # Helpers: ws.svelte.js (WS store), api.js (fetch wrappers)
│   │   └── assets/        # Images
│   ├── dist/              # Built SPA (gitignored; placeholder index.html committed)
│   ├── package.json       # Dependencies: Svelte, Vite, Vitest
│   └── vitest.config.js   # Test configuration
├── site/                  # Documentation site (static HTML generator)
│   ├── src/              # Markdown source
│   └── dist/             # Generated HTML
├── docs/                  # Architecture & user docs
│   ├── README.md         # Full specification (§1–§10)
│   ├── arch/             # Per-component design docs (piece A–K)
│   ├── user/             # UI walkthrough with screenshots
│   └── external/         # PLAYER.md (wire protocol), esp32.md
├── scripts/              # Build & dev tools
│   ├── build.sh          # Compile Go (multi-arch) + Svelte UI
│   ├── ui.sh             # Web UI build only
│   ├── check.sh          # Linting (go vet) + tests
│   ├── dev2.sh           # Two-node dev setup (loopback + null sinks)
│   └── e2e.sh            # Smoke test: discovery, grouping, playback
├── bin/                  # Built binaries (gitignored)
│   ├── ensemble_linux_amd64
│   ├── ensemble_linux_arm64
│   ├── ensemble_linux_armv6
│   └── ensemble (native)
├── docker/               # Multi-arch Dockerfile, compose examples
├── data/                 # Test/dev media + node state (gitignored)
│   ├── media/           # Sample audio files
│   ├── 001/, 002/       # Per-node dev state (node.json, cluster.json)
│   └── node.json        # Single-node test state
├── testdata/            # Fixtures for tests (committed)
│   └── media/           # Test WAV files
├── respot/              # go-librespot binaries (for Spotify Connect, pre-built)
│   ├── go-librespot_linux_x86_64
│   ├── go-librespot_linux_arm64
│   └── go-librespot_linux_armv6
├── .claude/             # Claude-generated codebase docs (auto-indexed)
├── .planning/           # Phase planning index (orchestrator-generated)
├── go.mod, go.sum       # Go dependencies
├── tools.go             # Build tool imports (no code)
├── IMPLEMENTATION.md    # Piece-by-piece breakdown (source of truth for integrator)
├── README.md            # User-facing quick start
├── QUICKSTART.md        # Docker setup & Spotify configuration
├── RELEASING.md         # Tag-based release process
├── verdict.md           # Current state & known issues
├── .gitlab-ci.yml       # CI pipeline (build, test, release)
├── .gitignore           # Committed .git(ignored)/node_modules/bin/data/web/dist
└── Dockerfile           # Multi-arch build target
```

## Directory Purposes

**`cmd/ensemble/`:**
- Purpose: Main binary entry point
- Contains: main.go (component wiring, port binding, lifecycle management), main_test.go (startup integration tests)
- Key files: `cmd/ensemble/main.go:56` (main), `cmd/ensemble/main.go:164` (run)
- Entry point for the orchestration layer

**`internal/contracts/`:**
- Purpose: Shared interface definitions (skeleton, piece S)
- Contains: Backend, Sink, Clock, StateStore, Snapshot DTOs
- Key files: `internal/contracts/contracts.go:1` (all interfaces defined here)
- Pure contract, no implementation; a leaf package (imports only id + stdlib)

**`internal/id/`:**
- Purpose: Node identity primitive
- Contains: ID [16]byte, New() (random), Parse(string), String() (hex), XOR(), JSON marshal/unmarshal
- Key files: `internal/id/id.go:1`, `internal/id/id_test.go`

**`internal/config/`:**
- Purpose: Configuration & node persistence (piece A)
- Contains: Flag/env parsing, node.json load/create, role (playback/master/combined), spotify endpoint config
- Key files: `internal/config/config.go:1` (Load), `internal/config/node.go:1` (persistence)
- Behavior: Defaults from ENSEMBLE_* env vars, flags, then hardcoded defaults. PORT bind-or-increment except when pinned.

**`internal/discovery/`:**
- Purpose: mDNS peer enumeration (piece B)
- Contains: zeroconf register/browse, TXT record parse (id/gossip/http/stream/source/control ports)
- Key files: `internal/discovery/discovery.go:1` (New, Peers), `internal/discovery/parse.go:1` (TXT parsing)

**`internal/cluster/`:**
- Purpose: Gossip state replication, group derivation (piece C)
- Contains: memberlist wrapper, LWW document, node/playback/group-name records, setters (SetName, SetVolume, SetFollowing), Snapshot() resolver
- Key files: `internal/cluster/cluster.go:1` (Cluster struct), `internal/cluster/doc.go:1` (LWW doc), `internal/cluster/store.go:45` (snapshot construction)

**`internal/stream/`:**
- Purpose: Wire protocol, UDP mux, subscriber transport (pieces S, G)
- Contains: wire.go (frame header codec, packet types), mux.go (UDP mux), client.go (subscriber FEC/reorder), control.go (stream control messages)
- Key files: `internal/stream/wire.go:1` (frame format), `internal/stream/mux.go:1` (UDP dispatcher), `internal/stream/client.go:61` (subscribe)

**`internal/audio/`:**
- Purpose: Media source factory, codec module (piece D)
- Contains: source.go (factory), decoders (file.go, http.go, flac.go, mp3.go, wav.go), input.go (line-in capture), opus.go (encode/decode), resample.go, live.go (generator), devices.go (input enumeration)
- Key files: `internal/audio/source.go:91` (Open), `internal/audio/opus.go:24` (Encoder/Decoder), `internal/audio/devices.go:1` (ListInputDevices)

**`internal/sink/`:**
- Purpose: Output backend registry, playout pipeline (piece E)
- Contains: backend_*.go (alsa/exec/null/file implementations), sink.go (scheduler + rate servo), jitter.go (jitter buffer), servo.go (PI controller), resampler.go (4-tap Catmull-Rom), gain.go (volume ramp), registry.go (named backend lookup)
- Key files: `internal/sink/sink.go:1` (Playout), `internal/sink/sink.go:177` (scheduler loop), `internal/sink/servo.go:1` (rate servo PI controller)

**`internal/clock/`:**
- Purpose: Master/follower clock sync (piece F)
- Contains: clock.go (Server + Follower), sample.go (offset tracker), payload.go (wire format)
- Key files: `internal/clock/clock.go:1` (Clock interface), `internal/clock/clock.go:230` (Follower.MasterNow)

**`internal/source/`:**
- Purpose: Source server, fan-out, FEC, subscriber registry (piece G)
- Contains: server.go (TCP+UDP listener, subscriber registry, fan-out), ring.go (release ring buffer), fec.go (XOR parity), prime.go (burst prime), registry.go (source stats registry)
- Key files: `internal/source/server.go:86` (Run), `internal/source/server.go:289` (frame fan-out)

**`internal/group/`:**
- Purpose: Group engine, playback orchestration, session management (piece H)
- Contains: engine.go (Engine struct), play.go (Play state machine), session.go (generation + restart handling), follow.go (membership change), settings.go (group settings), watch.go (cluster change listener), queuesource.go (file queue processor)
- Key files: `internal/group/engine.go:1` (Engine), `internal/group/play.go:79` (Play), `internal/group/engine.go:50` (Run)

**`internal/playback/`:**
- Purpose: Master-side control driver for remote playback nodes (piece D59-D62)
- Contains: driver.go (command sender), control.go (ATTACH/DETACH/SetVol/SetDelay messages), player.go (local in-process playout)
- Key files: `internal/playback/driver.go:1` (Driver), `internal/playback/control.go:1` (Control)

**`internal/spotify/`:**
- Purpose: Spotify Connect bridge management (piece D57)
- Contains: manager.go (multi-endpoint orchestration), bridge.go (go-librespot subprocess interaction)
- Key files: `internal/spotify/manager.go:1` (Manager), `internal/spotify/bridge.go:1` (Bridge)

**`internal/api/`:**
- Purpose: HTTP API, WebSocket, SPA serving, node proxy (piece I)
- Contains: api.go (Echo setup), handlers.go (REST endpoints), ws.go (WebSocket), proxy.go (node proxy), spa.go (SPA fallback), dto.go (JSON DTOs)
- Key files: `internal/api/api.go:1` (New), `internal/api/handlers.go:1` (route handlers), `internal/api/api.go:1` (websocket debounce)

**`web/`:**
- Purpose: Single-page application (piece J)
- Contains: Svelte 5 components, fetch wrappers, WebSocket store, media browser, group/node controls
- Key files: `web/src/App.svelte:1` (layout), `web/src/lib/ws.svelte.js:1` (WS store), `web/src/lib/api.js:1` (fetch wrappers)

**`docs/`:**
- Purpose: Architecture specification and design rationale
- Contains: README.md (full spec), arch/*.md (per-piece design docs), user/README.md (UI walkthrough)
- Key files: `docs/README.md` (source of truth), `docs/arch/` (numbered design documents D1–D65)

**`scripts/`:**
- Purpose: Build automation and development convenience
- Contains: build.sh (multi-arch compile), ui.sh (Svelte build), check.sh (vet + test), dev2.sh (local two-node setup), e2e.sh (smoke test)
- Key files: All executable; `scripts/build.sh` is the canonical build script

## Key File Locations

**Entry Points:**
- `cmd/ensemble/main.go:56` — Process main entry point
- `internal/api/handlers.go:1` — HTTP route handlers
- `internal/group/engine.go:50` — Group engine main loop
- `internal/sink/sink.go:177` — Playout scheduler loop
- `internal/source/server.go:86` — Source server fan-out loop

**Configuration:**
- `go.mod` — Dependencies (echo, memberlist, zeroconf, gorilla/websocket, opus/flac/mp3/wav decoders, purego)
- `internal/config/config.go:1` — Flag/env parsing
- `internal/config/node.go:100` — node.json structure (ID, name, volume, delay, outputDevice)

**Contracts & Interfaces:**
- `internal/contracts/contracts.go:1` — All cross-piece interfaces
- `internal/stream/wire.go:22` — Frame header codec, packet types
- `internal/id/id.go:1` — Node ID primitive

**Core Logic:**
- `internal/cluster/doc.go:1` — LWW document (group derivation, state replication)
- `internal/sink/servo.go:1` — Rate servo PI controller
- `internal/group/play.go:79` — Playback state machine
- `internal/audio/source.go:91` — Media source factory (Open)

**Testing:**
- `cmd/ensemble/main_test.go` — Startup integration tests
- `internal/sink/sink_test.go` — Jitter buffer, scheduler, servo tests
- `internal/audio/*_test.go` — Decoder/codec tests
- `web/src/lib/*.test.js` — UI helper tests

## Naming Conventions

**Files:**
- `*.go` — Go source files (internal packages + cmd/)
- `*_test.go` — Go test files (same package, run via `go test`)
- `*.svelte` — Svelte components (web/src)
- `*.js` — JavaScript helpers and utilities (web/src/lib)
- `.env*` — Environment variables (not committed; example: `.env.example`)

**Directories:**
- `internal/<piece>/` — One piece per directory; piece name matches IMPLEMENTATION.md
- `cmd/<binary>/` — One binary per directory
- `docs/arch/` — One numbered doc per piece (D1.md, D2.md, etc.)
- `web/src/components/` — Reusable Svelte components
- `web/src/sections/` — Page-level sections (Groups, Nodes)
- `web/src/lib/` — Shared JavaScript/helper modules

**Go Packages:**
- Piece packages export only essential types/functions (e.g., `cluster.New()`, `group.Engine`, `sink.Playout`)
- Internal types (e.g., `cluster.doc`, `sink.jitterBuffer`) unexported (lowercase)
- Interfaces in contracts are uppercase (Backend, Sink, Clock)
- Constructor functions follow pattern: `New(config) *Type` (e.g., `cluster.New`, `sink.New`)

**Variables & Functions:**
- CamelCase for exported symbols
- camelCase for local variables and unexported functions
- UPPERCASE for constants (especially protocol constants: `TypeAudio`, `FlagPrimeMe`)
- Receiver names: single letter (e.g., `func (c *Cluster) SetName()`) matching first letter of type

## Where to Add New Code

**New Feature (e.g., volume step control):**
- Primary code: `internal/group/settings.go` (if group-level) or `internal/cluster/setters.go` (if node-level)
- API endpoint: `internal/api/handlers.go` (add POST /groups/{id}/volup or similar)
- Wire message: `internal/stream/wire.go` (add packet type if cross-node, e.g., TypeSetVol)
- Tests: Co-located `*_test.go` files in the same package

**New Output Backend (e.g., PulseAudio):**
- Implementation: `internal/sink/backend_pulse.go`
- Registration: Add to `internal/sink/registry.go:48` (Registry.Open switch case)
- Tests: `internal/sink/backend_pulse_test.go` (mock output, verify Write calls)
- Capability detection: Update `cmd/ensemble/main.go:261` (capabilities()) if runtime-probed

**New Media Source (e.g., Bluetooth stream):**
- Implementation: `internal/audio/bluetooth.go`
- Factory registration: `internal/audio/source.go:91` (Open() switch case for "bluetooth://" scheme)
- Tests: Fixtures in `testdata/media/` or generated via test helpers
- No wire changes needed (all sources emit canonical PCM via ReadFrame)

**New Utility/Helper:**
- Shared helpers: `internal/netx/`, `internal/dl/`, etc. (leaf packages, minimal deps)
- Test helpers: Co-located in `*_test.go` files (not exported, not imported elsewhere)
- Web UI library: `web/src/lib/` (shared functions, test with vitest)

**New API Endpoint:**
- Handler function: `internal/api/handlers.go` (follows pattern of POST /groups/{id}/play)
- DTO (if needed): `internal/api/dto.go`
- Tests: `internal/api/handlers_test.go`
- Web UI: Add component to `web/src/components/` and wire into sections (Groups.svelte or Nodes.svelte)

## Special Directories

**`web/dist/`:**
- Purpose: Built SPA (index.html + app.js + app.css)
- Generated: Via `npm run build` (Vite)
- Committed: Placeholder index.html only (real build gitignored)
- Consumed: Embedded via `go:embed` in `internal/api` (api.go), served as SPA fallback

**`bin/`:**
- Purpose: Built binaries
- Generated: Via `./scripts/build.sh`
- Committed: No (gitignored)
- Contents: `ensemble_linux_{amd64,arm64,armv6}`, `ensemble` (native)

**`data/`:**
- Purpose: Runtime node state + test media
- Committed: Partially (media/ subdirectory with sample files; node.json gitignored)
- Structure: `data/media/` (user library), `data/001/`, `data/002/` (dev node state)

**`testdata/`:**
- Purpose: Test fixtures
- Committed: Yes
- Contents: Minimal WAV/MP3 files for decoder tests
- No sensitive data or large media

**`respot/`:**
- Purpose: Pre-built go-librespot binaries (Spotify Connect)
- Committed: Yes (LFS or direct, depending on repo size)
- Structure: `go-librespot_linux_{x86_64,arm64,armv6}`
- Consumed: Detected at startup if present, wrapped by `internal/spotify/bridge.go`

**`.planning/`:**
- Purpose: Orchestrator-generated phase index
- Committed: No (orchestrator writes this during execution)
- Contents: Phase implementations, codebase analysis

---

*Structure analysis: 2026-06-11*
