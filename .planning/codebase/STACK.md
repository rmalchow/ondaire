# Technology Stack

**Analysis Date:** 2026-06-11

## Languages

**Primary:**
- Go 1.26 - Backend binary, cluster coordination, audio processing, HTTP/WebSocket API
- JavaScript/Node.js (ES2022) - Web UI build tooling, marketing site generation

**Secondary:**
- HTML/CSS - SPA and marketing site markup/styling

## Runtime

**Environment:**
- Go 1.26 (Linux amd64/arm64 cross-compilation supported)
- Node.js 24 (Alpine-based images in CI)
- Go is statically compiled with `CGO_ENABLED=0` (pure Go, no C dependencies)

**Package Manager:**
- `go mod` - Go dependency management (`go.mod`, `go.sum` at root)
- `npm` - Node.js package management (v11.8.0 local, v24-alpine in CI)
- Lockfiles: `go.sum`, `web/package-lock.json`, `site/package.json` (no lockfile)

## Frameworks

**Core Backend:**
- Echo v4.15.2 - HTTP server, REST API routes (`/api/*`), WebSocket support via gorilla/websocket v1.5.3
- github.com/grandcat/zeroconf v1.0.0 - mDNS service discovery and advertisement
- github.com/hashicorp/memberlist v0.5.4 - Cluster gossip protocol for node discovery

**Frontend UI:**
- Svelte v5.0.0 - Component framework (`web/src/App.svelte`, embedded in Go binary)
- Vite v6.0.0 - Development server and build tool (dev proxy to http://localhost:8080, `npm run dev`)
- @sveltejs/vite-plugin-svelte v5.0.0 - Svelte integration with Vite

**Testing:**
- Vitest v2.1.0 - Unit test runner (`npm run test`, environment: jsdom)
- jsdom v25.0.0 - DOM implementation for browser testing

**Build/Dev:**
- Node.js build system - `web/package.json` scripts (dev, build, preview, test)
- Go build (`go build ./...`), cross-compile via `GOOS/GOARCH` flags
- Custom site generator - `site/build.mjs` (zero-dependency static site builder, Node.js)

## Key Dependencies

**Critical Backend:**
- github.com/labstack/echo/v4 v4.15.2 - HTTP routing and middleware
- github.com/gorilla/websocket v1.5.3 - WebSocket protocol support for live UI updates
- github.com/hashicorp/memberlist v0.5.4 - Gossip-based cluster membership
- github.com/grandcat/zeroconf v1.0.0 - mDNS discovery (Spotify Connect, peer discovery)

**Audio Processing:**
- github.com/hajimehoshi/go-mp3 v0.3.4 - MP3 decoding
- github.com/go-audio/wav v1.1.0 - WAV format support
- github.com/mewkiz/flac v1.0.13 - FLAC decoding
- github.com/ebitengine/purego v0.10.1 - Pure Go FFI to libc (syscall library)
- github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8 - Audio metadata reading

**Spotify Integration:**
- go-librespot v0.7.3 - External Docker image (ghcr.io/devgianlu/go-librespot:v0.7.3)
  - Bundled in Docker runtime; managed as subprocess bridge via `internal/spotify/`
  - Spotify metadata and PCM tap integration
  - Spotify Connect protocol (Zeroconf-advertised)

**Infrastructure/Network:**
- golang.org/x/net v0.53.0 - Advanced networking utilities
- golang.org/x/crypto v0.50.0 - Cryptographic primitives
- github.com/miekg/dns v1.1.68 - DNS client (mDNS support)

**Logging:**
- log/slog (Go stdlib) - Structured logging, default logger with key=value fields

## Configuration

**Environment Variables:**
- `ENSEMBLE_HTTP_PORT` - HTTP server port (default 8080)
- `ENSEMBLE_STREAM_PORT` - Audio stream port (default 9090)
- `ENSEMBLE_SOURCE_PORT` - Source subscription/control (default 9200)
- `ENSEMBLE_CONTROL_PORT` - Master→playback command channel (default 9300)
- `ENSEMBLE_GOSSIP_PORT` - Cluster gossip port (default 7946)
- `ENSEMBLE_ROLE` - Node role: "master" | "playback" | "master,playback" (default both)
- `ENSEMBLE_DATA_DIR` - Persistent state directory (default "data"; contains node.json, cluster.json, go-librespot auth)
- `ENSEMBLE_MEDIA_DIR` - Music library root (default ENSEMBLE_DATA_DIR/media)
- `ENSEMBLE_OUTPUT` - Audio backend: "" (auto) | "null" | "exec" | "file:<path>" | "alsa"
- `ENSEMBLE_LOG` - Log level: debug | info | warn | error (default info)
- `ENSEMBLE_HOST` - Bind address for HTTP/gossip (default all interfaces, "127.0.0.1" in dev/e2e)
- `ENSEMBLE_JOIN` - Dev gossip seed list: comma-separated host:port (overrides mDNS)
- `ENSEMBLE_NO_MDNS` - Disable mDNS discovery (dev/test: use --join instead)
- `ENSEMBLE_ALSA_LATENCY_MS` - ALSA hardware latency tuning (default 0, clamped ±500)

**Build Configuration:**
- CLI flags: `--http-port`, `--stream-port`, `--source-port`, `--control-port`, `--gossip-port`, `--role`, `--data`, `--media`, `--name`, `--join`, `--no-mdns`, `--host`, `-v` (verbose/debug)
- Flag precedence: CLI flags > environment variables > defaults (resolved in `internal/config/config.go`)

**Persistence:**
- `node.json` - Per-node identity, name, volume, output delay, disabled features, Spotify endpoints (JSON, located at `ENSEMBLE_DATA_DIR/node.json`)
- `cluster.json` - Cluster membership snapshot (JSON, located at `ENSEMBLE_DATA_DIR/cluster.json`)
- go-librespot auth - Cached under `ENSEMBLE_DATA_DIR/` (default endpoint) or `ENSEMBLE_DATA_DIR/spotify/<id>/` (preset endpoints)

## Platform Requirements

**Development:**
- Go 1.26+
- Node.js 24+ (or compatible npm v11+)
- Linux/Unix-like OS (tested on Linux, cross-compilation to amd64/arm64)
- Audio libraries (optional): libasound2 (ALSA), libopus, libdl (dynamic linking)

**Production:**
- Deployment target: Docker (multi-stage builds for amd64/arm64)
- Base image: ghcr.io/devgianlu/go-librespot:v0.7.3 (Alpine 3.21)
- Network: Host networking required for mDNS discovery and Spotify Connect advertisement
- Media storage: Mounted as read-only `/media`
- Data storage: Persistent volume at `/data` (node.json, cluster.json, go-librespot auth)
- Ports: HTTP (8080 base), stream (9090), source (9200), control (9300), gossip (7946) — bind-or-increment when taken

## Build Output

**Go Binary:**
- `ensemble` - Self-contained static binary (no runtime dependencies)
- Embedded SPA: Web UI from `web/dist/` built into binary via `go:embed`
- Cross-compiled to `ensemble-linux-amd64`, `ensemble-linux-arm64` per CI pipeline
- Version stamped via `-X main.version=…` at build time

**Web SPA:**
- `web/dist/` - Vite-built Svelte SPA (ES2022, relative asset paths)
- Embedded in Go binary; served at `/` via Echo
- Production entrypoint: `index.html` (committed placeholder pre-build)

**Marketing Site:**
- `site/dist/` - Zero-dependency static HTML/CSS/WOFF2/PNG
- Built by `site/build.mjs` (Node.js, no external tools)
- Fully static with relative paths (no server required)

---

*Stack analysis: 2026-06-11*
