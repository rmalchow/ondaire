# External Integrations

**Analysis Date:** 2026-06-11

## APIs & External Services

**Spotify Connect:**
- Service: Spotify (Spotify Client Library)
  - What it's used for: Playback control via Spotify's proprietary Connect protocol (premium users can control playback from any device on the network)
  - SDK/Client: go-librespot v0.7.3 (subprocess bridge)
  - Auth: Cached Spotify credentials stored in `ENSEMBLE_DATA_DIR/` or `ENSEMBLE_DATA_DIR/spotify/<endpoint-id>/` (go-librespot handles OAuth internally)
  - Integration: Managed via `internal/spotify/manager.go` and `internal/spotify/bridge.go`
  - Metadata: Live PCM tap + track metadata from go-librespot process (`internal/audio/spotify.go`)
  - Multiple endpoints: Support for Spotify Connect presets (multiple simultaneous endpoints per node)
  - URI scheme: `spotify:` (default) or `spotify:<endpoint-id>` (named presets)

## Data Storage

**Databases:**
- Not used - No external databases (PostgreSQL, MySQL, MongoDB, etc.)

**File Storage:**
- Local filesystem only
  - User library: Read-only mount at `/media` (containing audio files: MP3, FLAC, WAV)
  - Persistent state: `/data` volume (mutable)

**Caching:**
- In-memory cluster snapshot cache (`internal/cluster/`)
- go-librespot credential cache on filesystem (within `/data`)
- No Redis or external caching service

## Authentication & Identity

**Auth Provider:**
- Custom - No external identity provider
- Local node identity: Immutable node ID (UUID-like) persisted in `node.json`
- Node naming: User-configurable per node, persisted in `node.json`
- Spotify credentials: Managed internally by go-librespot (OAuth handled by go-librespot process)
- No API authentication for cluster nodes (trusts mDNS/gossip network)

**Implementation:**
- Cluster trust: Based on mDNS advertisement + gossip protocol (hashicorp/memberlist)
- API routes: No HTTP authentication required (`internal/api/` exposes public endpoints)
- Network security: Relies on network isolation (typically LAN-only deployment)

## Monitoring & Observability

**Error Tracking:**
- None configured - No external error tracking (Sentry, Rollbar, etc.)

**Logs:**
- Approach: Structured logging via Go stdlib `log/slog`
  - Output: stdout/stderr only
  - Level: Configurable via `ENSEMBLE_LOG` env var (debug | info | warn | error)
  - Format: Key=value pairs with component labels (e.g., `comp=api`, `comp=spotify-mgr`)
  - Fields: Request IPs, operation metrics, error details
  - Access logs: Optional request logging in `internal/api/middleware.go`

**Metrics:**
- Internal only: Sink stats (playout counters), cluster membership size, stream packet loss
- Exposed via `/api/status` endpoint (no external metrics collection)
- No Prometheus, CloudWatch, or datadog integration

## CI/CD & Deployment

**Hosting:**
- Docker - Multi-architecture images (amd64, arm64)
- Base image: ghcr.io/devgianlu/go-librespot:v0.7.3 (Alpine 3.21)
- Registry: harbor.rand0m.me/public/ensemble:latest (internal registry)

**CI Pipeline:**
- GitLab CI (`/.gitlab-ci.yml`)
  - Stages: frontend, test, build, release
  - Frontend: `npm ci && npm run build` (build Svelte SPA)
  - Test: `go vet`, `go test`, `gofmt` check, arm64 cross-compile smoke test
  - e2e: Optional manual loopback cluster test (`scripts/e2e.sh`)
  - Build: Cross-compile Linux amd64/arm64 binaries, parallel artifact builds
  - Release: Tag-triggered Docker multi-arch image push

**Deployment:**
- Docker Compose (example in `docker/docker-compose.yml`)
- Host networking required (no port publishing; mDNS and Spotify Connect need real LAN)
- Environment: Harbor registry for published images
- Orchestration: Supports both standalone Docker and Compose deployments

## Environment Configuration

**Required env vars:**
- `ENSEMBLE_DATA_DIR` - Persistent state directory (mandatory, no default in prod)
- `ENSEMBLE_MEDIA_DIR` - Music library path (default: $ENSEMBLE_DATA_DIR/media)
- `ENSEMBLE_ROLE` - Node role (optional: master|playback|master,playback, default both)

**Optional tuning env vars:**
- `ENSEMBLE_ALSA_LATENCY_MS` - Hardware latency calibration (default 0)
- `ENSEMBLE_AUDIO_OUTPUT` - Backend selection (default: auto-detect)
- `ENSEMBLE_HOST` - Bind address (default: all interfaces)
- `ENSEMBLE_LOG` - Log level (default: info)

**Secrets location:**
- Spotify auth credentials: Stored in `$ENSEMBLE_DATA_DIR/` (local filesystem)
- No API keys or secrets in environment variables
- No external secrets management (HashiCorp Vault, AWS Secrets Manager, etc.)

## Webhooks & Callbacks

**Incoming:**
- None - No inbound webhooks supported

**Outgoing:**
- None - No webhook calls to external services

## Network & Discovery

**Service Discovery:**
- mDNS (Multicast DNS) - Primary: Zeroconf-based peer discovery
  - Advertised service: `_ensemble._tcp` for cluster nodes
  - Spotify Connect service: Advertised by go-librespot via Zeroconf
  - Disable: `ENSEMBLE_NO_MDNS=1` (dev/test, use `ENSEMBLE_JOIN` instead)

**Cluster Communication:**
- Gossip Protocol: hashicorp/memberlist (custom on `ENSEMBLE_GOSSIP_PORT`)
  - Peer discovery via mDNS or seed list (`ENSEMBLE_JOIN`)
  - Cluster state replication (membership, node metadata)
  - No external coordination service (Consul, etcd, etc.)

**Data Streams:**
- RTP-based audio streaming: Custom protocol on `ENSEMBLE_STREAM_PORT` (§8.1)
  - Unicast (point-to-point) between master and playback nodes
  - FEC (Forward Error Correction) for packet loss resilience
  - No external streaming service

**Source Subscriptions:**
- Internal control on `ENSEMBLE_SOURCE_PORT`
  - Master→playback control commands on `ENSEMBLE_CONTROL_PORT`
  - No external API calls for playback coordination

## Media Sources

**Supported Protocols:**
- `file://` - Local filesystem via `ENSEMBLE_MEDIA_DIR`
- `http://` / `https://` - HTTP(S) streaming (audio files, streams, playlists)
- `spotify:` / `spotify:<id>` - Spotify playback via go-librespot bridge
- `input:` - Live ALSA input (microphone/line-in; when available)

**HTTP Client:**
- Standard library `net/http` with connection pooling (`internal/audio/http.go`)
- No external HTTP client library (httpx, resty, etc.)
- Timeout: Not explicitly set globally (relies on connection defaults)

## Format Support

**Audio Codecs (Decoding):**
- MP3 - via github.com/hajimehoshi/go-mp3
- FLAC - via github.com/mewkiz/flac
- WAV - via github.com/go-audio/wav
- Opus - via dynamic linking (libopus, optional, dlopen at runtime)
- Spotify - via go-librespot

**Metadata:**
- ID3, Vorbis, FLAC - via github.com/dhowden/tag

**Output Encoding:**
- Master: Opus (when available) for inter-node streaming
- Playback: ALSA (when available) or null sink

## External Process Integration

**go-librespot:**
- Launched as subprocess by `internal/spotify/bridge.go`
- Binary location: Injected at runtime (CI builds `bin/ensemble-linux-*` with go-librespot on PATH)
- IPC: Custom bridge API (internal protocol on TCP socket)
- Lifecycle: Started/stopped per Spotify endpoint; active endpoint preempts others
- Failure mode: Bridge restart on connection loss; user notified via cluster state update

---

*Integration audit: 2026-06-11*
