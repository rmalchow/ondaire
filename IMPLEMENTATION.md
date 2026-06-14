# Building & contributing

How the codebase is organized and the conventions it holds to. For the system
design itself, read the [architecture docs](docs/architecture/); for the user-facing
view, the [user guide](docs/user/).

## Package map

```
cmd/ensemble/        main: flag parsing, capability probing, wiring, lifecycle
cmd/player/          standalone reference player (pure stdlib; proves the player protocol)
cmd/soundcheck/      local tone / bring-up tool
internal/id/         node/group IDs (gen, parse, XOR, JSON)
internal/config/     flags + env, data dir, node.json, role + CONTROL_PORT
internal/dl/         runtime shared-library probe (purego dlopen/dlsym; soft-fail)
internal/netx/       bind-or-increment listeners, CIDR interface scan
internal/discovery/  mDNS register + browse (role-aware TXT, player announce)
internal/cluster/    memberlist wrapper, replicated LWW doc, observed IPs, derivation
internal/clock/      clock server + follower (UDP via the stream mux)
internal/source/     audio source server: subscriber registry, ring buffer, prime, STATUS
internal/stream/     frame wire codec + packet types, member-side UDP mux, subscriber client
internal/audio/      media sources (file/http/input), decoders, resampler, opus (runtime libopus)
internal/playback/   the player role: Player verb interface + control listener + master-side driver
internal/sink/       playout engine (jitter buffer, fractional resampler, phase-lock servo);
internal/sink/device/  the device port (Sink + optional capabilities) and adapters (alsa/exec/file/null)
internal/api/        Echo server: REST, WebSocket, node proxy, SPA embed
internal/contracts/  cross-package interface + DTO types (Sink, Clock, stats, …)
web/                 Svelte 5 SPA (vite; npm run build → web/dist, embedded via go:embed)
docs/                documentation (user / architecture / developer / reference)
```

The shared contracts in `internal/contracts`, `internal/stream/wire.go`, `internal/id`,
`internal/netx`, and `internal/dl` are the seams every other package builds against;
change them deliberately, since the blast radius is wide.

## Ground rules

- **Simple and basic.** No speculative abstraction, no feature that isn't in the
  design. Prefer 200 lines that obviously work over 600 that might.
- **One universal binary.** No cgo, no build tags. Optional native libraries
  (`libopus`, `libasound`) are probed at runtime via `internal/dl` and soft-fail to a
  reported "capability off" — the same `linux/amd64` / `linux/arm64` build runs on a
  desktop or a Raspberry Pi.
- **Standard library first.** Current third-party deps: echo v4, hashicorp/memberlist,
  grandcat/zeroconf, gorilla/websocket, hajimehoshi/go-mp3, mewkiz/flac, go-audio/wav,
  ebitengine/purego (via `internal/dl` only).
- **Tests run without privileges.** No network root, audio hardware, or external files:
  loopback sockets, the null backend, a fake clock, a skewed fake DAC, generated WAV
  fixtures, an httptest server. Opus round-trips `t.Skip` when libopus isn't loadable.
- **Logging** is `log/slog`, component-scoped (`slog.With("comp", "cluster")`).

## Checks

After a change, the gate is:

```sh
go build ./... && go vet ./... && go test ./...
./scripts/check.sh      # the same, plus the UI build
```

`./scripts/dev2.sh` launches two local nodes (null sinks) to try grouping/playback by
hand; `./scripts/e2e.sh` runs the end-to-end smoke test (discovery → grouping →
takeover → synchronized playback → late-join prime → RESTART recovery → stop), and also
builds the reference player to exercise the [player protocol](docs/developer/player-protocol.md).
