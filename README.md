# ensemble

**Self-organizing multiroom audio.** One small binary per device. Start it on
two machines and they find each other, form a group, and play the same music
in sync — no server, no database, no config files, no cloud.

```
┌── kitchen ──┐   ┌── living room ──┐   ┌── bedroom ──┐
│  ensemble   │◀─▶│    ensemble     │◀─▶│  ensemble   │
└─────────────┘   └────────┬────────┘   └─────────────┘
                           │ plays a track from its library,
                           ▼ streams it in lock-step to the whole group
                     🔊  🔊  🔊   one song, every room, in time
```

- **Zero-config** — nodes discover each other over mDNS + gossip and replicate
  their state automatically. Pull the plug on any node and the rest heal.
- **One universal binary** — no cgo, no build flavors. Opus and ALSA are loaded
  at runtime if present, so the same `linux/amd64` or `linux/arm64` build runs
  on a beefy desktop or a Raspberry Pi.
- **Synchronized playback** — a master-anchored clock plus a per-node rate servo
  keep rooms aligned; lost packets are recovered with FEC or TCP.
- **Built-in web UI** — every node serves a single-page app at `/`. Control the
  whole cluster from any one of them.
- **Plays your stuff** — local files (`wav` / `mp3` / `flac`), internet radio
  (`http(s)://…`), or a line-in capture. Browse any node's library from any UI.
- **Group as you like** — any node is a group of one by default; tell one to
  follow another and they merge. Name a combination of rooms and the name
  sticks whenever that set reforms.

> A node can also be a receive-only **player**: the wire protocol is small and
> documented in [`docs/developer/player-protocol.md`](docs/developer/player-protocol.md) for firmware
> (e.g. an ESP32 + DAC), with a reference player in [`cmd/player`](cmd/player).

---

## Quick start

### Build

Needs Go 1.26 and Node (for the UI). Pure-Go, statically-ish linked:

```sh
git clone https://gitlab.rand0m.me/share/ensemble.git
cd ensemble
./scripts/build.sh --ui     # builds the SPA, then linux amd64 + arm64 into bin/,
                            # plus ./ensemble for this host
```

Prebuilt binaries are attached to each [tagged release](RELEASING.md).

**Running in Docker?** A multi-arch master image (amd64 + arm64, Spotify Connect
built in) plus a Compose example live in [`docker/`](docker/) — see the
[Docker quick start](QUICKSTART.md).

### Run

On each device:

```sh
./ensemble
```

That's it — defaults are sensible. The node mints a permanent ID on first run,
binds its ports, advertises itself over mDNS, and starts serving the UI. Run it
on a second machine on the same LAN and they find each other within seconds.

Open the UI in a browser:

```
http://<that-device's-ip>:8080
```

Put music in the node's library directory (created on first run):

```
<data-dir>/media/        # default: ./data/media — folders are browsed recursively
```

### Two nodes on one machine (to try it out)

```sh
./scripts/dev2.sh        # launches two nodes on 127.0.0.1 with null audio sinks
```

…or run the full end-to-end smoke test:

```sh
./scripts/e2e.sh
```

---

## Using the UI

Every node serves the same app and proxies to the others, so open
`http://<any-node-ip>:8080` on a phone or browser. There are two pages: the
**Rooms** overview (landing page) and the **Nodes** page (the **⚙ gear**,
top-right).

- **Rooms** — one card per group: now-playing (cover art / title / artist),
  group + per-member volumes, and — when you select a card — the add-players
  roster, the media browser, and advanced per-group settings (codec/transport/
  buffer).
- **Nodes** — per-node cards: addresses, feature toggles (playback/opus/input,
  plus a **spotify** badge), **Spotify endpoints** (Connect devices + presets),
  and settings (volume, hw delay, output device, test tone).

📖 **Full walkthrough with screenshots: [docs/user](docs/user/README.md).**

<p>
  <img src="docs/user/images/overview.png" width="240" alt="Rooms overview" />
  <img src="docs/user/images/nodes.png" width="240" alt="Nodes page" />
</p>

---

## Configuration

Flags (each has an `ENSEMBLE_*` env equivalent); all are optional:

| Flag | Default | What |
|------|---------|------|
| `--data <dir>` | `./data` | data dir: `node.json`, `cluster.json`, `media/` |
| `--media <dir>` | `<data>/media` | library directory (browsed recursively) |
| `--name <name>` | first 8 of the node ID | display name (first start only) |
| `--http-port` | `8080` | UI + REST API + WebSocket + node proxy |
| `--stream-port` | `9090` | member-side stream + clock sync (tcp+udp) |
| `--source-port` | `9200` | audio source: subscriptions + control (tcp+udp) |
| `--gossip-port` | `7946` | cluster gossip (tcp+udp) |
| `--host <addr>` | all interfaces | bind address |
| `--output <spec>` | `auto` | output backend: `auto` \| `alsa` \| `exec` \| `null` \| `file:<path>` |

Ports are **bind-or-increment** *when left at the default*: if one is taken the
node tries the next and advertises whatever it actually bound. A port you **set
explicitly** (flag or `ENSEMBLE_*_PORT` env) is **pinned** — it binds that exact
number or the node exits with a clear error, so a misconfiguration surfaces
immediately instead of silently drifting.

Env-only knobs:

| Env | Default | What |
|-----|---------|------|
| `ENSEMBLE_ALSA_LATENCY_MS` | `200` | ALSA device buffer (raise on a glitchy Pi) |
| `ENSEMBLE_LOG` | `info` | log level (`debug` \| `info` \| `warn` \| `error`) |

`auto` picks the best available backend: **alsa → exec** (`pw-play`/`aplay`/…)
**→ null**. Pass `-v` for debug logging; `ensemble run …` is accepted as an
alias for `ensemble …`.

For a hermetic setup without mDNS (e.g. tests, or a segmented network), use
`--no-mdns` with `--join <host:gossipPort>,…` seeds.

---

## How it works (in one breath)

Every node runs the identical binary and is, by default, the master of a group
containing only itself. Telling node B to *follow* node A merges them; the only
replicated fact is each node's `following` field — group membership, the group
ID (the master's ID), and failover all fall out of that, recomputed everywhere
from gossiped state. When a group plays, its master opens the media source,
stamps each 20 ms PCM/Opus frame with a presentation time on the shared clock,
and streams it to every member (including itself) that has *subscribed* to its
source port. Each member runs a clock follower and a jitter-buffered, rate-
servo'd playout so the audio leaves every speaker at the same instant.

Deeper docs live in [`docs/`](docs/), organized into four groups: the
[user guide](docs/user/), the [architecture reference](docs/architecture/)
(topic pages — wire protocol, clock sync, playout pipeline, sequence diagrams, …),
the [developer docs](docs/developer/) (the [player wire protocol](docs/developer/player-protocol.md),
the [ESP32 hardware node](docs/developer/esp32.md), and
[acoustic calibration](docs/developer/calibration.md)), and a
[reference section](docs/reference/) of external prior art.
[`RELEASING.md`](RELEASING.md) covers releases.

## Measured coherence

Sync is easy to claim, so we measured it. With a single USB microphone in the
room and two Raspberry-Pi players (`pi01` → left, `pi02` → right) we recorded the
real acoustic gap between the speakers over a 12-minute run.

**How it's measured.** Each speaker plays a wideband log-sine sweep, time-
interleaved L/R so the two never overlap. The recording is matched-filtered
against the reference sweep with sub-sample parabolic interpolation to pin each
arrival. The trick that makes it robust: every `pi02` sweep is referenced to the
**midpoint of its two bracketing `pi01` sweeps**, which cancels the (large,
common) playback-vs-microphone clock rate exactly and leaves only the
`pi01`↔`pi02` offset. As a ground-truth check, a deliberate ~50 cm speaker move
was recovered as 46 cm from the audio alone. The toolkit is pure Python in
[`tools/calib/`](tools/calib/) (`lr_drift.py`, `compare_drift.py`); the full
write-up is in [`docs/developer/calibration.md`](docs/developer/calibration.md).

### Inter-speaker offset: 716 µs RMS

The two speakers held to **716 µs RMS**, peak-to-peak ~2.7 ms. The curve is a
smooth thermal "bowl" (sound-card crystals warming up), not fast jitter. The
precedence/Haas effect fuses two arrivals into one perceived source up to ~5–10
ms, so this stays well inside the single-source range — sub-millisecond most of
the session, with a slow multi-ms thermal excursion the rate-servo doesn't cancel.

### What the servo can't see

Polling the master's per-node STATUS telemetry (`GET /api/playback/statuses`)
during the same run, the players' self-reported clock-offset difference stays
nearly flat while the microphone sees the full ~2.7 ms wander — correlation
**r = +0.20**. The drift lives **downstream of the clock the servo controls**, in
the DAC/analog path, where the chain is currently blind to it. That gap is small
enough to be inaudible, but it's exactly what on-demand acoustic calibration
(`outputDelayMs`) is designed to measure and null.

## Status & scope

Works today: discovery, grouping, takeover, synchronized playback, pause/resume,
opus, per-node volume/delay/device, persisted names & settings, the web UI, and
a tag-driven release pipeline. Out of scope for now: auth/TLS, internet-facing
operation, playlists, and hardware-mixer volume. v1 is a trusted-LAN system.

## On AI

Yes — I used Claude for much of this code. No, it isn't vibe-coded AI slop. The
architecture, the clock-sync math, the wire protocol, and the trade-offs behind
every design decision are my own thinking and experience, and I know and
understand pretty much every line that ships. I used AI the way you'd use any
good power tool: to get past my own laziness and move faster, not to outsource
the judgement. If something in here is wrong, that's on me, not on a model.
