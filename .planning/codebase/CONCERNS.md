# Codebase Concerns

**Analysis Date:** 2026-06-11

## Architectural Constraints & Limitations

### Trusted-LAN Security Model

**Risk:** No authentication or TLS. Any peer on the network can control the cluster and blast audio to any device.

**Files:** `internal/api/ws.go:37-38` (explicit comment: `// Trusted LAN, SPA served same-origin; allow any origin (§9 no auth)`), `internal/api/handlers.go`, `README.md:177-179` (explicit scope: "v1 is a trusted-LAN system")

**Current mitigation:** Documentation is clear and honest about the constraint. Design is intentional, not an oversight.

**Recommendations:** 
- For production use, operators must deploy on isolated VLAN or behind firewall
- Document enterprise deployment scenarios requiring authentication layer (reverse proxy with auth)
- Flag as breaking change for future v2 with auth

### Linux-Only Audio Output

**Problem:** Platform support is restricted to Linux for audio playback. No native macOS or Windows audio backend.

**Files:** `internal/sink/backend_alsa.go` (ALSA), `internal/sink/backend_exec.go` (exec wrapper for pw-play/aplay), `scripts/build.sh:22-26` (Linux-only GOOS)

**Impact:** The system can cross-compile binaries for ARM/x86, but they are useless without ALSA or fallback tools (pw-cat/aplay). Desktop users running macOS or Windows cannot use ensemble as a speaker endpoint.

**Scaling path:** Add native backends for:
- macOS: CoreAudio, PulseAudio
- Windows: DirectSound or WASAPI

## FEC Weakness Under Burst Loss

**What happens:** Forward Error Correction uses XOR parity over fixed blocks of 4 frames. Recovers exactly ONE lost frame per block.

**Files:** `internal/stream/fec.go:8-9` (constant `fecBlockSize = 4`), verdict.md:90-94 (architect's own analysis)

**Why it matters:** WiFi packet loss is burst-not-random. A 2+ frame burst loss in a 4-frame window blows through FEC recovery. The fallback is TCP (which trades latency for certainty), but users on lossy networks may see audio gaps.

**Actual behavior:** System detects FEC insufficiency and falls back to TCP per D13. Not a silent failure, but users see degraded experience.

**Improvement path:**
- Consider Reed-Solomon FEC (recovers multiple lost frames per block) at cost of overhead
- Add packet loss telemetry to guide users toward TCP fallback
- Profile TCP latency impact on sync precision

## Dependency on Reverse-Engineered go-librespot

**Risk:** Spotify Connect integration uses go-librespot, a reverse-engineered client riding Spotify's ToS goodwill.

**Files:** `internal/spotify/bridge.go`, `internal/spotify/manager.go`, `go.mod` (indirectly via respot/ binaries)

**Impact:** 
- Spotify may revoke connect device support at any time
- go-librespot binaries (arm64/amd64/armv6) are pre-compiled and vendored; no source in this repo
- Updates require manual re-builds of go-librespot

**Current state:** Spotify integration is "nice to have," not core to the product promise.

**Recommendations:** 
- Mark as "experimental" in docs
- Monitor go-librespot releases and upstream changes
- Prepare fallback (URL radio streams only, no Connect)

## mDNS Discovery Brittleness

**What happens:** The "zero config" first-run relies entirely on mDNS multicast. On real home networks with segmentation, mesh systems, or IoT VLANs, mDNS fails silently.

**Files:** `internal/discovery/discovery.go`, `README.md:34-41` (acknowledged in the text)

**Impact:** 
- Nodes don't discover each other on segmented networks
- Recovery path (`--no-mdns --join <hosts>`) is documented but non-obvious to users
- verdict.md:34-41 flags this explicitly: "mDNS is exactly the thing that falls over on real home networks"

**Current state:** Design is honest but first-run experience is overstated by marketing ("discover within seconds"). Escape hatch exists.

**Improvement path:**
- Add mDNS fallback to link-local unicast probes
- Auto-detect mDNS failure and suggest `--join` in UI
- Better first-run diagnostics

## Single-Maintainer Bus Factor

**Risk:** This is a one-person project on a personal GitLab instance, not GitHub with stars/issues/community.

**Files:** README.md (no CONTRIBUTING guidelines), verdict.md:42-45 (skeptic's assessment)

**Impact:** 
- No backup maintainer if author becomes unavailable
- Community contributions harder to coordinate
- Long-term project sustainability unclear

**Mitigation:** Code quality + architecture discipline is high (per verdict.md:54-84); codebase is readable enough for adoption if needed.

**Recommendation:** Mirror to GitHub for visibility; establish contributor guidelines if accepting PRs.

## Test Coverage Gaps

**What's not tested:**
- End-to-end multi-master failover under packet loss
- Clock sync precision under real network jitter (currently 1 Hz probes, precision claims untested at >1 meter distance)
- ALSA backend failure recovery under sustained xrun load
- Opus codec edge cases (invalid bitrates, truncated streams)

**Files:** 
- `internal/sink/sink_test.go:359-360` (smoke test on push after reset, no panic)
- `internal/stream/client_test.go:305` (RESTART retry tested, but not under high loss)
- `internal/clock/clock_test.go:325` (clock reply handled, no panic, but not precision measurement)

**Priority:** Medium. Unit tests cover happy paths and fault tolerance. Integration tests (`scripts/e2e.sh`) are manually triggered (not in release gate), so real multi-node behavior under stress is untested at CI time.

**Path:** Add time-boxed load tests to e2e pipeline; measure clock sync jitter at different network conditions.

## Goroutine Leak Risk in Stream Subscriber

**Pattern:** Subscriber goroutines in `internal/stream/client.go:runMux` / `runWatchdog` are spawned for each session and are guarded by `closed` flag and context cancellation.

**Files:** `internal/stream/client.go:300-390`, `internal/group/watch.go:100-150`

**Potential issue:** If a subscriber is created, starts the mux/watchdog goroutines, then the group transitions away (master change, session stop) before the context is canceled, goroutines may linger briefly. The `closed` flag prevents crashes but message dispatch becomes a no-op.

**Current state:** Cleanup is gated on context cancellation and channel close; WaitGroup tracking ensures shutdown waits. The pattern is sound but fragile if callers don't cancel context.

**Safe modification:** Always ensure `Cancel()` is called on stream subscriber contexts; verify in group engine watch loop (`group/watch.go:line 182` BYE handler).

## Sink Backend Rotation Under Load

**Problem:** When the active ALSA backend fails (xrun, device disconnect), `resilientBackend.Write()` rotates to the next backend in the chain.

**Files:** `internal/sink/backend_resilient.go:98-151`

**Issue:** The error is swallowed (`return nil` at line 151). If all backends fail (ALSA hangs, exec missing), the frame is silently dropped. No metric surfaces this to the user.

**Current state:** The null backend is always in the chain (fallback), so audio never stops—it just goes nowhere. This is safe but means silent degradation.

**Impact:** A user with a misconfigured ALSA device may play audio with no indication that output is unavailable.

**Improvement path:**
- Add a "last resort" alert if the fallback backend is active for >N seconds
- Surface backend rotation in status API / UI

## Missing Rate-Limiting on API Endpoints

**What:** The HTTP API has no rate limiting. A malicious peer (or buggy loop) can hammer endpoints.

**Files:** `internal/api/handlers.go`, `internal/api/api.go:82` (no rate-limit middleware)

**Current state:** On trusted LAN this is acceptable (§1, architecture decision). Not a bug, a scope choice.

**Implication:** If ensemble is ever exposed to untrusted networks, rate limiting is critical.

**Recommendation:** Document as prerequisite for internet-facing deployment; add rate limiter skeleton (no-op by default) in API init for future use.

## Cluster State Reconciliation Edge Case

**Pattern:** After a network partition, nodes converge via gossip with eventual consistency. If a group master is in partition A and a member transitions to partition B, the member may follow a stale master for up to the push/pull cycle (~5s).

**Files:** `internal/cluster/cluster.go:64` (authoritative observed map), `docs/arch/DECISIONS.md:D7` (restart version bump rule)

**Current state:** Self-heal timer (grace period) fires after ~10 seconds of stale observation (D45). Audio stops cleanly; user clicks "leave group" or master rejoins.

**Edge case:** If the partition heals during the grace period, the member may re-sync to the old master without a clean stop/restart, causing brief audio glitches or duplicate playback.

**Impact:** Low. Partitions are rare on LANs. Audio glitches are short-lived.

**Safe modification:** Group engine already handles mid-session master transitions via takeover (`group/watch.go:line 72`). No change needed.

## ALSA Xrun Recovery Logging

**Issue:** Xrun events are logged at WARN but throttled (logged once every 10 seconds max to avoid spam).

**Files:** `internal/sink/backend_alsa.go:150-165` (xrun throttling)

**Problem:** If xruns are frequent (every few seconds), the throttle hides the severity. A user's ALSA device is failing but only one warning every 10 seconds appears.

**Current state:** By design—xrun logs would spam at kHz rates otherwise. Throttle is reasonable.

**Improvement:** Expose cumulative xrun counter in status API / UI so users can see the magnitude of the problem.

## Concurrent Map Access in Cluster State

**Pattern:** The cluster node records map is guarded by a single `mu sync.Mutex`. High-frequency snapshot reads (WebSocket fanout) and writes (gossip updates) contend on the same lock.

**Files:** `internal/cluster/cluster.go:100-120` (snapshot under lock), `internal/cluster/delegate.go:200-220` (gossip merge under lock)

**Current state:** Contention is acceptable for typical cluster sizes (<20 nodes). Go's sync.Mutex is fast for uncontended paths.

**Scaling limit:** Beyond ~100 nodes on a saturated home WiFi, snapshot latency may increase. RwMutex could help, but current lock is held for short periods (JSON unmarshal only for large updates).

**Path for scaling:** Profile at larger cluster sizes; consider read-only replica if fanout becomes a bottleneck.

## Opus Codec Unavailability Silent Degradation

**What:** If libopus is not available at runtime, Opus is silently disabled (capability off).

**Files:** `internal/audio/opus.go:34-60` (probe), `go.mod` (no libopus dependency—loaded at runtime via dl.Open)

**Impact:** If a node is configured to use Opus but libopus is missing (misconfigured container, missing package), the node falls back to PCM instead of returning an error.

**Current state:** This is intentional (D32)—the same binary runs everywhere, degrading gracefully if libraries aren't present.

**Risk:** A user enables Opus in UI, thinks it's active, but libopus is missing. They get PCM instead. No error surfaced.

**Mitigation:** Status API includes codec availability; UI shows capability (D3 capability per piece K).

## HTTP Client Timeout Not Enforced on Streaming Responses

**Pattern:** The audio source (HTTP) streams infinite bodies (radio streams, line-in capture).

**Files:** `internal/audio/http.go:14-16` (ResponseHeaderTimeout set, but Timeout = 0)

**Why:** Setting an overall Timeout would kill legitimate infinite streams. Only the response header timeout is bounded (10s dial/header arrival).

**Risk:** If an HTTP stream source server stalls after sending headers (no data flow for >1 hour), the connection hangs indefinitely.

**Current state:** Acceptable for LAN use. On internet-facing deployment, add a per-frame read deadline.

## Clock Follower Cold-Start Tuning

**Pattern:** On startup, a node bursts clock probes (~10–20 over 200ms) before settling to 1 Hz (D53).

**Files:** `internal/clock/clock.go:300-330` (startup burst), `docs/arch/DECISIONS.md:D53`

**Potential:** If the master is slow to respond (200ms+ RTT), the burst window may not capture a response, forcing fallback to unsync'd playout until the next 1 Hz request succeeds.

**Current state:** Design accepts the trade-off (fast sync-up vs. guaranteed convergence). On real LANs, RTT is <10ms so the burst converges.

**Improvement path:** Add backoff if burst fails; increase burst window if median RTT is high.

## Missing Graceful Shutdown Coordination

**Pattern:** When ensemble exits (SIGINT/SIGTERM), components are torn down in reverse order of initialization (`internal/config/config.go` → cluster → group → sink).

**Files:** `cmd/ensemble/main.go:76-150`, `internal/group/engine.go:140-160` (Stop method)

**Potential:** If an HTTP request is in-flight when the cluster is closed, the API handler may reference stale cluster state, causing a crash.

**Current state:** The `Close()` path is defensive (nil checks throughout), but no explicit "drain in-flight requests" logic.

**Risk:** Low—the shutdown sequence is short (<100ms typically), so races are rare.

**Safe modification:** Add a small grace window in main.go to let in-flight HTTP requests complete before cluster teardown.

## Playback Queue Mutation Under Concurrent Access

**Pattern:** The playback queue (QueueSource) can be modified while a frame is being read.

**Files:** `internal/group/queuesource.go:200-260` (ReadFrame with URI guard), `internal/group/play.go:72-140` (concurrent Play/Stop/Next mutations)

**Safeguard:** URI guard checks that the item's URI hasn't changed mid-frame (line 251). If it has, the frame is dropped and RESTART fires.

**Edge case:** If the queue is cleared (all items deleted) while ReadFrame is mid-buffer, the guard prevents a crash but the frame is lost. Not a data corruption issue, just a dropped frame.

**Current state:** Design is sound—the guard exists for this reason. Calls to Skip/Pause/Stop all drop the current frame cleanly.

## Networking Path MTU Discovery Not Used

**Issue:** Ensemble sends audio frames in UDP datagrams without MTU discovery. Frames are 3840 bytes (20 ms @ 48kHz s16le stereo); typical Ethernet MTU is 1500, so frames are fragmented.

**Files:** `internal/stream/transport.go` (UDP send), `internal/stream/wire.go:20` (FrameBytes = 3840)

**Impact:** Fragmentation + reassembly overhead on receivers; if any fragment is lost, the whole frame is lost (no reassembly at frame level).

**Why:** Opus packets (~100–300 bytes) are sent as-is (no fragmentation). Audio frames are always split anyway.

**Improvement:** Could add optional "large frame" UDP bundling for Opus (e.g., 10 frames per datagram), but current design avoids the complexity.

## Missing Performance Baselines

**Issue:** No documented performance profiles or load tests.

**What's missing:**
- Max nodes in a cluster before gossip/snapshot latency exceeds audio deadlines
- CPU/memory per 10-node cluster
- Latency impact of different transport codecs (Opus vs. PCM over TCP)

**Files:** `.planning/codebase/` (no PERFORMANCE.md)

**Current state:** verdict.md:99-103 acknowledges sync precision claims are untested ("I'd want to actually hear two speakers a meter apart").

## Installer Role / Privilege Escalation (Recent Issue)

**Status:** Partially addressed in recent commit `67f2959 feat(installer): optional headless-appliance hardening`

**Files:** `scripts/installer.sh` (not in internal/, checked separately), recent commits show hardening work

**Implication:** The installer can gain elevated privileges for system configuration. Recent changes suggest this was a vulnerability concern that's been mitigated.

**Current state:** Hardening is in place. No further action needed unless new privilege paths are added.

## Documentation Drift Risk

**Pattern:** The spec lives in `docs/README.md`, architecture decisions in `docs/arch/DECISIONS.md`, but implementation details in code comments are the ground truth.

**Files:** Scattered `// D<number>` citations throughout codebase (e.g., `internal/stream/fec.go:3-4` references `D8.4`, but decision file is `§8.4`)

**Risk:** Decision citations in code use mix of notations (`§` vs. `D` vs. numbers vs. filenames), making it hard to correlate code to spec.

**Improvement:** Standardize decision citation format (e.g., always `// D13 TCP stream framing`). Audit 10% of random files to ensure spec alignment.

## Shutdown Order Assumptions

**Pattern:** Main's shutdown sequence assumes components are independent; no explicit contract about teardown order.

**Files:** `cmd/ensemble/main.go:85-120` (init order reversed on shutdown), `internal/group/engine.go:Line 90` (Stop calls cluster shutdown internally)

**Potential:** If a component's Stop() method calls back into another component during shutdown, deadlocks are possible.

**Current state:** No callbacks observed in Stop() methods (checked grep for "defer" + calls during Close). Safe.

---

*Concerns audit: 2026-06-11*
