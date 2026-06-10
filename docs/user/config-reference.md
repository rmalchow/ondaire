# Ensemble — Configuration Reference

> **You are here:** [User Guide](README.md) › **Configuration Reference**
> This page documents every knob ensemble has, with the *why* behind each one.
> For where controls live in the app, see the [UI Reference](ui-reference.md). For
> step-by-step setup, see the [scenarios](README.md#pick-your-setup).

Ensemble is designed to need **no configuration** — `./ensemble` with no arguments
is the intended way to run it on most machines. Everything below is optional, for
when you want to pin a port, point at a library elsewhere, restrict a node to one
role, or understand a setting you found in the UI.

There are two layers of configuration, and it's worth keeping them straight:

- **Startup configuration** (this page, sections 1–6) — flags and environment
  variables read **once at launch**. These set identity, directories, roles,
  ports, discovery, and the audio backend. Change them by restarting the node.
- **Live, persisted settings** (sections 7–8) — per-node and per-group knobs you
  change **from the web UI at runtime**. They're saved to disk and survive
  restarts; you rarely edit them by hand.

---

## 1. How options are resolved

Every startup option can be given as a command-line **flag** or an
**`ENSEMBLE_*` environment variable**. Precedence, highest first:

```
flag  >  environment variable  >  built-in default
```

So a flag always wins over the matching env var, and either wins over the default.

**Native binary** — pass flags, or export env vars:

```sh
./ensemble --name kitchen --role playback
# equivalently:
ENSEMBLE_ROLE=playback ./ensemble --name kitchen
```

**Docker** — env vars via Compose `environment:` (or `-e`), and/or flags **after**
the image name:

```sh
docker run --network host -e ENSEMBLE_LOG=debug \
  harbor.rand0m.me/public/ensemble:latest --name living-room
```

> `ensemble run …` is accepted as an alias for `ensemble …`, and `-v` is shorthand
> for `ENSEMBLE_LOG=debug`. For what the banner and log lines mean — including the
> per-second clock & playback fields — see [Debugging](debugging.md).

---

## 2. Identity & directories

| Flag | Env | Default | What it does |
|------|-----|---------|--------------|
| `--name <name>` | — | first 8 hex of the node ID | The node's **display name**, shown in the UI and used to derive its Spotify Connect device name (`ensemble <name>`). **Applied on first start only** — afterwards, rename from the Nodes page (the name is persisted and replicated). |
| `--data <dir>` | `ENSEMBLE_DATA_DIR` | `./data` (relative to the working dir) | The **data directory**: holds `node.json` (this node's identity + settings), `cluster.json` (replicated cluster state), and — if Spotify is used — go-librespot's credentials and audio FIFO. Must be **writable** and should **persist across restarts/upgrades** so the node keeps its identity and Spotify login. |
| `--media <dir>` | `ENSEMBLE_MEDIA_DIR` | `<data>/media` | The **library directory**, browsed **recursively** in the UI. Point it at your existing music folder. Read-only is fine — ensemble only ever reads from it. |

**Identity.** On first start a node mints a permanent random ID and writes
`node.json`. That ID — not the name or IP — is the node's stable identity; you can
rename it or move it to a new IP and it's still the same node. Keep the data dir
and the node keeps its identity.

---

## 3. Roles

A node runs one or both roles. Set with `--role` / `ENSEMBLE_ROLE`:

| Value | Meaning |
|-------|---------|
| *(unset)* / `both` / `all` | **Both roles** (the default): can host a library, source/stream audio, **and** play out a speaker. Right for a desktop or a Pi-with-speakers. |
| `master` | **Master only**: owns cluster state, serves the UI/API, sources and streams audio — but **never plays out a local speaker**. Right for a headless NAS/server. (The Docker image defaults to this.) |
| `playback` | **Player only**: receives a stream and plays it; can't host a library or become a group master. Right for a thin speaker node. |

The value is case-, order-, and whitespace-insensitive; `master,playback` and
`playback master` are the same. A node with no role does nothing, so an empty set
is an error.

> Role is **runtime** configuration (not replicated). The *advertised* role goes
> into the node's mDNS announcement so peers know what it can do.

---

## 4. Ports

| Flag | Env | Default | Purpose |
|------|-----|---------|---------|
| `--http-port` | `ENSEMBLE_HTTP_PORT` | `8080` | UI + REST API + WebSocket + node-to-node proxy |
| `--stream-port` | `ENSEMBLE_STREAM_PORT` | `9090` | member-side stream + clock sync (TCP+UDP) |
| `--source-port` | `ENSEMBLE_SOURCE_PORT` | `9200` | audio source: subscriptions + stream control (TCP+UDP) |
| `--control-port` | `ENSEMBLE_CONTROL_PORT` | `9300` | master→playback commands |
| `--gossip-port` | `ENSEMBLE_GOSSIP_PORT` | `7946` | cluster gossip / membership (TCP+UDP) |

### Bind-or-increment vs. pinned

This is the one subtlety worth understanding:

- A port **left at its default** is **bind-or-increment**: if `8080` is taken, the
  node tries `8081`, then `8082`, and advertises whatever it actually bound. This
  is what lets you run **several nodes on one machine** without configuring
  anything.
- A port you **set explicitly** (via flag or env) is **pinned**: it binds that
  exact number or the node **exits with a clear error**. So a real
  misconfiguration surfaces immediately instead of silently drifting to a port
  nobody expects.

The startup banner always prints the **actual** bound ports.

> **Docker note:** because ports bind-or-increment and discovery is multicast,
> the image expects `--network host` and publishes no ports. See the
> [NAS / server guide](scenarios/nas-master.md#why-host-networking).

---

## 5. Discovery & clustering

Nodes find each other automatically over **mDNS** (multicast on the LAN) and then
gossip their state. There's nothing to configure for the normal case — they appear
in each other's UI within seconds of starting.

| Flag | Env | What it does |
|------|-----|--------------|
| `--no-mdns` | `ENSEMBLE_NO_MDNS=1` | **Disable mDNS** entirely (no announce, no browse). For hermetic tests or networks where multicast is blocked. You must then seed the cluster manually with `--join`. |
| `--join <host:gossipPort>,…` | `ENSEMBLE_JOIN` | **Seed list** of peers to gossip with directly, comma-separated (e.g. `192.168.1.10:7946`). Used together with `--no-mdns`, or to bridge across subnets. |

> mDNS is the recommended path on a normal home LAN. Reach for `--no-mdns`/`--join`
> only when multicast doesn't work (some managed switches, VLANs, or Docker
> bridge mode — see the [NAS guide](scenarios/nas-master.md#bridge-mode-fallback)).

---

## 6. Audio output backend

How a **player** gets sound out of the box it runs on. Env-only:

| Env | Default | What it does |
|-----|---------|--------------|
| `ENSEMBLE_OUTPUT` | `auto` | Selects the output backend (see below). |
| `ENSEMBLE_ALSA_LATENCY_MS` | `200` | ALSA device buffer in ms. **Raise it** (e.g. `400`) if a Pi crackles or underruns; lower it for tighter latency on a solid machine. |

`ENSEMBLE_OUTPUT` values:

- **`auto`** *(default)* — pick the best available backend, in order:
  **alsa → exec → null**. This is what makes "just run it" work everywhere.
- **`alsa`** — output directly to ALSA (the normal path on Linux/Raspberry Pi).
  Choose the specific device per-node from the UI (see
  [output device](#8-per-node-settings-nodejson)).
- **`exec`** — pipe PCM to an external player command (`pw-play`, `aplay`, …).
  Useful on a desktop running PipeWire/PulseAudio.
- **`null`** — discard audio. The node is a fully functional cluster member that
  makes no sound — handy for a master-only box, or for testing.
- **`file:<path>`** — write the raw PCM to a file. Diagnostics only.

The startup banner reports which backend was selected and, for Spotify, whether a
go-librespot binary was found (see [§9](#9-go-librespot-resolution)).

---

## 7. Per-group stream settings

These live on a **room (group)**, not a node, and you change them from the
**Advanced settings** twirl-down on a selected room
([UI Reference](ui-reference.md#room-controls-a-selected-room)). They apply live to
the whole group and are remembered for that set of rooms.

| Setting | Values | Default | What it trades off |
|---------|--------|---------|--------------------|
| **Codec** | `opus` · `pcm` | `opus` | **`opus`** compresses each 20 ms frame into one small packet — robust over Wi-Fi and the right default. **`pcm`** is uncompressed: zero codec latency and bit-exact, but larger packets that can fragment on Wi-Fi. Use `pcm` only on solid wired links. |
| **Transport** | `udp` · `tcp` | `udp` | **`udp`** is low-latency; lost packets are recovered with forward-error-correction (FEC) parity, so the occasional drop is inaudible. **`tcp`** guarantees delivery at the cost of a little latency and a hard stall if the link is truly congested. Switch to `tcp` only when UDP is being dropped wholesale. |
| **Buffer** | 50–500 ms | 150 ms | The per-group **playout buffer** that absorbs network jitter. **Raise it** on flaky Wi-Fi to stop dropouts (at the cost of more delay between "press play" and sound); **lower it** on a clean network for snappier response. |

**Rule of thumb:** leave these alone. If a room on Wi-Fi stutters, first raise the
**buffer**; `opus`/`udp`/FEC already handle most loss. Reach for `tcp` last.

---

## 8. Per-node settings (`node.json`)

These are **per-node**, set from the **Settings** section of a node's card on the
Nodes page ([UI Reference](ui-reference.md#the-nodes-page--gear)), persisted to
`node.json`, and replicated so any node's UI shows them. You normally never touch
the file directly.

| Setting (UI label) | `node.json` field | Range / values | What it does |
|--------------------|-------------------|----------------|--------------|
| **vol** | `volume` | `0.0`–`1.0` | The node's software output gain. Persists across restarts. |
| **hw delay** | `outputDelayMs` | `0`–`150` ms in the UI (clamped ±500 internally) | See [below](#hw-delay--alignment). |
| **output device** | `outputDevice` | ALSA device id; default `default` | Which sound card/output this node uses (on hosts with more than one). |
| *feature toggles* | `disabled` | subset of `playback`, `opus`, `input` | Operator-disabled capabilities. Toggle the tri-state chips in the **Features** section to turn a capability off on a given host (e.g. disable `input` on a node with no usable line-in). |
| *(grouping)* | `following` | a node ID, or empty for solo | The node's last-known follow target, so it **rejoins its previous room automatically** after a reboot. Managed entirely by the UI's add/remove-player actions. |
| *Spotify presets* | `spotifyEndpoints` | list of presets | The extra Spotify Connect devices this node advertises, edited in the **Spotify endpoints** section. See the [UI Reference](ui-reference.md#spotify-endpoints). |

### hw delay & alignment

Different speakers and amplifiers add different fixed delays of their own (DSP,
Bluetooth-style buffering, a slow DAC). When two rooms are *almost* in sync but one
trails the other by a hair, **hw delay** nudges that node's output earlier or later
to compensate that fixed device latency, so the rooms line up perfectly. It only
corrects a *constant* offset — ensemble's clock sync and rate servo already handle
drift over time. Hovering the label in the UI shows the same explanation; changing
it causes a brief local restart of that node's output.

> **hw delay** corrects a fixed, constant offset by hand. Automatic acoustic
> alignment is a separate, more advanced capability tracked in the developer docs.

---

## 9. go-librespot resolution

Spotify Connect is provided by an external **go-librespot** binary. At startup,
ensemble looks for it (then for `librespot`) in this order:

1. **The working directory** — an executable `./go-librespot` next to where you
   launched ensemble.
2. **`$PATH`** — anywhere on the system path.

If neither is found, Spotify is simply disabled (the banner says so) and everything
else works normally. Where to get the binary and how to deploy it per setup is in
[Spotify Connect & podcasts](scenarios/nas-master.md#spotify-connect--podcasts).
The official Docker image **bakes go-librespot in**, so Spotify works there with no
extra steps.

---

## Quick recipes

```sh
# A headless NAS master, library on an existing folder, verbose logs:
ENSEMBLE_ROLE=master ENSEMBLE_MEDIA_DIR=/srv/music ENSEMBLE_LOG=debug ./ensemble --name house

# A Raspberry Pi player with a roomier ALSA buffer (it was crackling):
ENSEMBLE_ALSA_LATENCY_MS=400 ./ensemble --role playback --name bathroom

# Two throwaway nodes on one laptop to try grouping (null audio, auto-incrementing ports):
ENSEMBLE_OUTPUT=null ./ensemble --data /tmp/n1 --name a &
ENSEMBLE_OUTPUT=null ./ensemble --data /tmp/n2 --name b &
```

---

**Next:** [UI Reference →](ui-reference.md) · [Back to the User Guide →](README.md)
