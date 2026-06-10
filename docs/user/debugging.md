# Debugging & reading the logs

> **You are here:** [User Guide](README.md) › **Debugging**
> See also: [Configuration Reference](config-reference.md) · [Running ensemble](running.md)

ensemble logs to **stderr** as structured `key=value` lines, preceded by a one-time
**banner**. This page explains the banner, the events you'll see during normal
operation, and how to read the per-second **clock** and **playback** fields when a
room won't sync or stutters.

Where the logs land depends on how you started the node:

| How it runs | Where to look |
|-------------|---------------|
| foreground | the terminal |
| systemd | `journalctl -u ensemble -f` |
| Docker | `docker logs -f <container>` |
| `nohup … >ensemble.log` | the file you redirected to |

## Log levels

One knob, `ENSEMBLE_LOG` (or the `-v` shorthand):

```sh
ENSEMBLE_LOG=debug ./ensemble     # debug | info (default) | warn | error
./ensemble -v                     # same as ENSEMBLE_LOG=debug
```

`info` is the normal level: lifecycle events plus a 1 Hz stats line **only while
audio is actually playing** (an idle node is quiet). `debug` adds the clock probe
loop, per-frame decode/drop reasons, and discovery/gossip chatter. See the
[Configuration Reference](config-reference.md#1-how-options-are-resolved).

Every log line carries a **`comp=`** tag naming the component that emitted it —
handy for filtering:

`main` · `clock-server` · `clock-follower` · `mux` · `stream-client` · `cluster` ·
`discovery` · `group` · `sink` · `source` · `spotify` · `pb-driver` / `pb-control` /
`pb-remote` (the master↔playback control plane) · `deliver` · `audio` · `api`.

```sh
journalctl -u ensemble -f | grep 'comp=sink'      # just the playout stats
```

---

## The startup banner

Printed once to stderr **regardless of log level** — it's the at-a-glance "what am
I, where, and what can I do":

```
═══════════════════════════════════════════════════════════════
  ensemble v0.3.6 — ready
───────────────────────────────────────────────────────────────
  node      kitchen  (a1b2c3d4e5f6…)
  roles     master,playback
  bind      0.0.0.0
  ports     http=8080  stream=9090  source=9200  gossip=7946  (tcp+udp; bind-or-increment)
  paths     data=./data  media=./data/media
  output    alsa
  codecs    pcm, opus
  sources   file, http, https, input
  backends  alsa, exec, null
  spotify   /usr/local/bin/go-librespot  (Connect device: "ensemble kitchen")
═══════════════════════════════════════════════════════════════
```

| Row | Meaning |
|-----|---------|
| **node** | display name + the permanent node ID (minted on first run, lives in `node.json`). |
| **roles** | `master`, `playback`, or `master,playback` — what this process does. See [roles](config-reference.md#3-roles). |
| **bind** | the bind address; `0.0.0.0` = all interfaces. |
| **ports** | the **actually bound** ports. With defaults these are bind-or-increment, so a busy port means the node took the next one — this line is the source of truth (see [ports](config-reference.md#4-ports)). |
| **paths** | resolved data dir (ID, cluster state, Spotify creds) and media library. |
| **output** | the **resolved** audio backend: `alsa`, `exec`, `null`, or `file`. `null` here on a node you expected to make sound means no usable device was found (or `--role master`). |
| **codecs** | `pcm` always; `opus` only if libopus was loadable at startup. No `opus` ⇒ groups fall back to PCM (much higher bitrate — rough on Wi-Fi). |
| **sources** | the URI schemes this node can open as a source (`file`, `http(s)`, `input` line-in). |
| **backends** | the output backends compiled/available on this host. |
| **spotify** | the resolved `go-librespot` path + advertised Connect device, or `not found (Spotify Connect disabled)`. See [Spotify Connect](spotify.md). |

A **playback-only** node (`--role playback`) prints a shorter banner — `roles
playback`, a `control=… stream=…` ports line, no `sources`/`spotify` — because it
only ever receives and plays.

> **First thing to check when something's off:** re-read the banner. Wrong `output`
> (`null` where you wanted `alsa`), missing `opus`, a port that incremented away
> from the default, or `spotify not found` explain most "it ran but didn't do what
> I expected" cases before you even reach the log lines.

---

## What's logged during normal operation (info)

At `info` you'll see a node move through its lifecycle. The common events:

**Startup** — `starting` → one `port bound` per socket → `output devices` /
`input devices` → `mDNS registered` → `ready`.

**Discovery & cluster** — `node joined` / `node left` (a peer appeared/expired over
gossip), `cluster state loaded`, `purged stale records`.

**Grouping** (`comp=group`) — `group composition` (initial view), `group player
joined` / `group player left`, `play target changed` (the group *this node's
speakers* follow), `unfollowing (now solo)`.

**A session starting/stopping** — `opening source` → `source opened` → `codec
negotiated` (pcm vs opus for this group) → `session start`; then `playback started`
/ `paused` / `resumed` / `playback ended (EOF)` / `session stop`. On the member
side: `subscribed` / `client joined (HELLO)` / `attached`, and `client left (BYE)` /
`unsubscribed` on teardown.

**Spotify** (`comp=spotify`) — `spotify bridge started`, `spotify playing`,
`spotify stop`.

**Master↔playback control** (`comp=pb-driver`) — `playback node discovered`,
`playback node assignment`, `room sync`, `room equalized` (cross-room buffer
alignment), `issued RESTART (starvation)`.

### The 1 Hz "playing" line

While audio is flowing, each node emits **one `playing` line per second** (silent
when idle). There are two flavours, told apart by `side=`.

**Master / source side** (`comp=group`, `side=master`) — what this node is
*sourcing* to the group:

```
playing side=master gen=7 released=512 clients=3 parity=128 restarts=0 primes=3
```

| Field | Meaning |
|-------|---------|
| `gen` | session generation; bumps on every new track / settings change / renegotiation. |
| `released` | frames handed to the network so far this session (each frame = **20 ms**, so 50/s). |
| `clients` | live subscribers right now — should equal the number of speakers in the group. |
| `parity` | FEC parity datagrams emitted (one per 4 audio frames on UDP/opus). |
| `restarts` | times a member asked to be re-primed after falling behind — a few at join is normal; a steady climb means a member can't keep up. |
| `primes` | burst re-sends of recent frames sent to (re)joining/recovering members. |

**Member / playout side** (`comp=sink`, `side=member`) — what this node is *playing*.
This is the line to read when a room misbehaves:

```
playing side=member played=3000 silence=2 lateDrop=0 buffered=10 ratePPM=18.4 \
        synced=true offsetNs=412000 rttNs=900000 delivered=3010 recovered=4 lost=0
```

*Clock fields* (how well this node agrees with the master on time):

| Field | Meaning · what to watch |
|-------|-------------------------|
| `synced` | the clock follower has a usable offset. **`false` ⇒ playout is gated** (you get silence) until it locks. Stuck `false` = the node can't reach the master's clock (port/firewall/`--join`). |
| `offsetNs` | this node's estimated clock offset from the master, in ns. On a synced LAN it's the real wall skew (µs–low ms); ≈0 on the master itself. A *stable* value is what matters, not a small one. |
| `rttNs` | smallest round-trip to the master in the recent window, ns. High or wildly varying RTT = a congested/flaky link, which widens the sync error — the cue to prefer wired or raise the buffer. |

*Playback fields* (how cleanly the audio is coming out):

| Field | Meaning · what to watch |
|-------|-------------------------|
| `played` | frames written to the sound card (20 ms each). Should advance by ~50/s while playing. |
| `silence` | silent frames inserted to fill gaps (underrun). A few at startup is normal; **climbing `silence` = frames aren't arriving in time** (loss or CPU starvation). |
| `lateDrop` | frames that arrived *past* their play deadline and were dropped. **Climbing `lateDrop` = buffer too small or the clock isn't holding** — raise the group buffer or `ENSEMBLE_ALSA_LATENCY_MS`. |
| `buffered` | current jitter-buffer depth, in frames. Should hover near the configured buffer; trending toward 0 precedes underruns. |
| `ratePPM` | the rate-servo's current micro-correction for sound-card clock drift, in ppm. Healthy: settles to a small **steady** value (tens of ppm). Pegged near ±300 or swinging = the servo is "hunting" — usually a too-small buffer or a very jittery device. |
| `delivered` | frames the subscriber received from the source. |
| `recovered` | frames lost on the wire but **rebuilt from FEC parity** — non-zero on Wi-Fi is normal and healthy. |
| `lost` | frames lost **and not** recovered (FEC handles only single-frame loss per block of 4). **`lost` > 0 = audible gaps** → switch the group transport to **TCP**, or raise the buffer. |

> **DeviceDelayNs / PhaseErrNs / Calibrated** appear in `/api/status` and in the
> control-plane STATUS from playback nodes (not the `playing` line). The master uses
> `DeviceDelayNs − PhaseErrNs` once `Calibrated` is true as each room's stable
> buffering setpoint, which is what the `room equalized` log reflects.

---

## Reading debug logs

`ENSEMBLE_LOG=debug` (or `-v`) adds the detail you need when the info line points at
a problem but not its cause:

**Clock probe loop** (`comp=clock-follower`, msg=`stats`) — one line per probe
interval:

```
stats synced=true offsetUs=412 rttUs=900 samples=8 gen=3 probes=240 replies=238
```

- `offsetUs` / `rttUs` — the same offset/RTT as above, in µs.
- `samples` — probes currently in the estimation window (more = a steadier estimate).
- `probes` vs `replies` — sent vs answered. A growing gap (`probes ≫ replies`) means
  the master isn't answering clock requests — the node will read `synced=false` and
  stay silent. Check that the stream port is reachable to the master.

**Decode / deliver** (`comp=deliver`) — `opus decoder created` once per stream;
`opus decode failed, dropping frame` or `opus disabled on this node, dropping frame`
per bad/again-disabled frame. A flood here explains a node that's subscribed but
silent.

**Discovery / gossip** (`comp=discovery`, `comp=cluster`) — peer adverts seen and
join/leave churn. Useful when two nodes *don't* find each other: if you never see
the other node's advert, mDNS isn't crossing your network — fall back to
`--no-mdns --join <host:gossipPort>` (see
[discovery](config-reference.md#5-discovery--clustering)).

### Quick symptom → field map

| Symptom | Look at | Likely fix |
|---------|---------|------------|
| Room is silent, others play | `synced` (false?), `delivered` (advancing?) | clock/port not reachable; check stream port & `--join` |
| Stutters / dropouts | `silence`, `lateDrop` climbing; `lost` > 0 | raise group buffer / `ENSEMBLE_ALSA_LATENCY_MS`; keep codec on **opus**; try **TCP** transport |
| Rooms slowly drift apart | `ratePPM` (pegged/swinging?), `offsetNs` (unstable?) | jittery link or buffer too small; prefer wired, raise buffer |
| Wrong/no sound device | banner `output` row (`null`?) | set `ENSEMBLE_OUTPUT` / pick the device in the UI |
| Phone can't see the node | banner `spotify` row; `comp=spotify` logs | install go-librespot ([Spotify Connect](spotify.md)); host networking under Docker |

The Wi-Fi tuning dials (group buffer, ALSA latency, opus vs TCP) are covered in the
[Raspberry Pi scenario](scenarios/raspberry-pi.md#picking-the-right-output--tuning-sync)
and the [Configuration Reference](config-reference.md#6-audio-output-backend).

---

**See also:** [Configuration Reference](config-reference.md) ·
[Running ensemble](running.md) · [UI Reference](ui-reference.md) ·
[User Guide](README.md)
