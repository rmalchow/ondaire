# The player protocol — receive-only, master-driven audio

This is a **self-contained** implementer's spec for an ensemble **player**: a
receive-only participant (a Go playback-only daemon, or ESP32 firmware) that plays
a group's audio in sync. You do not need to read any other document to implement
it. The standalone reference implementation is
[`cmd/player/main.go`](../../cmd/player/main.go) (pure stdlib, no `internal/` imports —
it proves this spec is sufficient); the production implementation is the in-binary
`Player` (`internal/playback`, run as `ensemble --role playback`).

> **Two roles, one binary.** An ensemble node runs two independently-enableable
> roles: a **room** (the `master`
> role — gossips, owns cluster state, serves the API + SPA, sources audio, and
> *drives* players) and a **player** (the `playback` role — this document). A node
> can run a room, a player, or both. The player protocol is **identical for Go and
> MCU**; they differ only in announced **capabilities** (§5). "room"/"player" are
> the user-facing names; the wire and CLI keep `role=master` / `role=playback`.

---

## 1. Scope — what a player is and is NOT

A player:

- **announces itself over mDNS** (§5) so masters can discover it — but it
  **never gossips** and holds **no cluster state**;
- is **driven by a master** over a small control plane (§6): a master tells it to
  `ATTACH` to a stream, sets its volume / output-delay, toggles capabilities, and
  reads its `STATUS`. It is **idle until ATTACHed**;
- once attached, **subscribes** to the master's audio source and **follows** the
  master clock so playout is sample-aligned with the group's real members;
- is **discovered and represented by a master**: the master injects a
  non-gossiping node record for it, the operator assigns it to a group (it reuses
  the `Following` field), and it then **shows up in the cluster + UI** like any
  member — assignable, volume/delay-controllable, health-monitored. It is **never a
  solo group of its own** and never becomes a master;
- is **invisible to codec negotiation**: it does not appear in the gossiped
  `groups[].members` quorum that picks the codec. The group picks its codec from
  its **real (gossiping) members** only. A master MAY *warn* when assigning a
  player whose announced codecs (§5) don't cover the group codec, but the codec is
  not changed for it.

**This is the v2 model.** It replaces an earlier self-directed design where a
client polled `GET /api/cluster` and chose its own master (and an even earlier
"thin HTTP API on the device" sketch — neither shipped). Self-direction now
survives **only** as a bench/bring-up fallback in the standalone reference and the
MCU's opt-in fixed-master mode (§11); a *deployed* player is always
mDNS-discovered, master-driven, and visible.

Because you are invisible to negotiation, **you cannot rely on the group running a
codec you support.** The default group codec is **opus**. A protocol-minimal player
typically wants **PCM** (no decoder) — so either the group is pinned to `pcm`, or
you implement an **opus** decoder (libopus). The `ATTACH` (§6) tells you which codec
to expect; refuse/warn if you can't decode it.

**Transport choice.** Raw PCM is `24 + 3840 = 3864` bytes per frame, which
**IP-fragments into ~3 UDP packets**; losing any fragment loses the whole frame, and
the per-frame XOR FEC (one parity per 4 frames) often cannot recover it on lossy
Wi-Fi. So: wired/reliable links → PCM over UDP is fine; Wi-Fi → use **opus** (a 20 ms
packet ≈ 320 B, one datagram, never fragments) **or** TCP transport (retransmits; no
FEC). The master chooses the transport and tells you in `ATTACH`.

---

## 2. Protocol version (v2)

**The magic byte `0xE5` IS the version marker.** Every framed packet begins with it.

1. **Unknown packet `type` values MUST be ignored by receivers** (forward
   compatibility — new optional types may be added within v2).
2. A **future incompatible** revision changes the **magic byte**, not the layout. A
   receiver that sees a leading byte other than `0xE5` MUST drop the datagram.

> v2 adds the control-plane types (`0x30`–`0x41`, §6) and the player's mDNS
> announcement (§5) to the v1 data plane. The data-plane wire (header, audio, FEC,
> clock, HELLO/BYE/RESTART/RECONFIG) is unchanged from v1.

---

## 3. The common 24-byte header

Every packet — audio, FEC, clock, and control — starts with this fixed header.
**All multi-byte fields are big-endian.**

| Offset | Size | Field        | Type   | Meaning                                          |
|-------:|-----:|--------------|--------|--------------------------------------------------|
| 0      | 1    | `magic`      | u8     | always `0xE5`                                    |
| 1      | 1    | `type`       | u8     | packet type (table below)                        |
| 2      | 4    | `gen`        | u32 BE | session generation; drop audio frames with `gen < yours` |
| 6      | 8    | `seq`        | u64 BE | frame sequence number, 0-based per session       |
| 14     | 8    | `pts`        | i64 BE | presentation timestamp, **master-clock ns**      |
| 22     | 2    | `payloadLen` | u16 BE | payload byte count following the header          |
| **24** |      |              |        | **total header size**                            |

The payload follows immediately. Over UDP, one datagram = one header + payload. Over
TCP, each frame is **length-prefixed**: a `u32 BE` byte-count, then exactly that many
bytes (which themselves begin with the 24-byte header). On control packets
(`0x2x`/`0x3x`/`0x40`/`0x41`) `gen`/`seq`/`pts` are unused unless stated; set them to 0.

### Packet types

**Data plane** (master's SOURCE_PORT / STREAM_PORT — unchanged from v1):

| Type   | Name        | Direction      | Player        | Notes                                  |
|--------|-------------|----------------|---------------|----------------------------------------|
| `0x01` | AUDIO       | src → sub      | **required**  | header + PCM (3840 B) or opus payload  |
| `0x02` | FEC         | src → sub      | optional      | XOR parity over 4 audio frames (UDP)   |
| `0x10` | CLOCK_REQ   | sub → master   | **required**  | you send these (clock probe)           |
| `0x11` | CLOCK_RSP   | master → sub   | **required**  | you receive these (clock reply)        |
| `0x20` | HELLO       | sub → src      | **required**  | subscribe / keepalive; flag prime-me   |
| `0x21` | BYE         | sub → src      | optional      | "leaving, stop sending"                |
| `0x22` | RESTART     | sub → src      | recommended   | "I got lost, re-prime me"              |
| `0x23` | RECONFIG    | src → sub      | **required**  | "session/settings changed"; flag stop  |

**Control plane** (the player's **CONTROL_PORT**, and STATUS back to the
master's SOURCE_PORT — **new in v2**):

| Type   | Name      | Direction        | Player        | Notes                                          |
|--------|-----------|------------------|---------------|------------------------------------------------|
| `0x30` | ATTACH    | master → player  | **required**  | "join this stream"; payload §6.1               |
| `0x31` | DETACH    | master → player  | **required**  | "leave, go idle"; no payload                   |
| `0x32` | SETVOL    | master → player  | recommended   | volume + mute; payload §6.2                     |
| `0x33` | SETDELAY  | master → player  | recommended   | output-delay (ms, signed); payload §6.2         |
| `0x34` | SETCAP    | master → player  | optional      | enable/disable a capability; payload §6.2       |
| `0x35` | SETEQ     | master → player  | optional      | cross-room equalization delay (ms, unsigned, added); payload §6.2 |
| `0x40` | STATUS    | player → master  | recommended   | telemetry; payload §6.3                         |
| `0x41` | STATUSREQ | master → player  | recommended   | liveness poll "send STATUS now"; no payload |

**Mandatory:** AUDIO, CLOCK_REQ/RSP, HELLO, RECONFIG, ATTACH, DETACH.
**Recommended:** RESTART, SETVOL, SETDELAY, STATUS, STATUSREQ.
**Optional:** FEC, BYE, SETCAP.

**Data-plane control payload (`0x20`/`0x22`/`0x23`):** a single byte flag.
- HELLO / RESTART: bit0 = **prime-me** (`0x01`) → request a burst of recent frames.
- RECONFIG: bit0 = **stop** (`0x01`) → the session ended.

---

## 4. Ports and addressing

A player binds **two** sockets:

- **CONTROL_PORT** — a **fixed UDP port it advertises over mDNS** (§6). The master
  sends `ATTACH`/`DETACH`/`SETVOL`/`SETDELAY`/`SETCAP` here. Default `9300`.
- **audio/clock socket** — a single `udp4` socket bound to an **ephemeral** port,
  used for *all* of: sending HELLO/BYE/RESTART, sending CLOCK_REQ + reading
  CLOCK_RSP, and reading AUDIO/FEC/RECONFIG.

**Key UDP detail (observed-by-construction):** over UDP, the master streams audio
back to **the exact source address your HELLO came from**. So you MUST send HELLO and
clock probes from the **same** socket you read audio on (one ephemeral socket). The
master learns your audio return address from your HELLO — the CONTROL_PORT is only
for master→playback commands, never for audio.

On `ATTACH` the master gives you its **`sourceEndpoint`** (send HELLO/BYE/RESTART
here, and TCP-dial here) and its **`clockEndpoint`** (send CLOCK_REQ here, receive
CLOCK_RSP). Send your `STATUS` (§7.3) to the master's `sourceEndpoint` (the master
reads control there). Over TCP transport, audio + data-plane control ride the one
TCP connection to `sourceEndpoint`; the clock still uses UDP to `clockEndpoint`.

---

## 5. mDNS announcement + capabilities

A player advertises the standard `_ensemble._tcp` service (domain `local.`) with a
TXT record. Masters browse this service to discover players. A player advertises
**no gossip port** (it never joins memberlist). The mDNS **instance name is the node
`id`** (32 hex), stable across reboots, so a master always re-discovers the same
player under the same key; the friendly label rides the `name` TXT key.

TXT keys:

| Key       | Example                  | Meaning                                                   |
|-----------|--------------------------|-----------------------------------------------------------|
| `id`      | `4ed795d4…` (32 hex)     | this node's immutable 16-byte id, hex-encoded             |
| `role`    | `playback`               | `playback`, `master`, or `master,playback` (combined)     |
| `control` | `9300`                   | CONTROL_PORT (master→player commands)                     |
| `name`    | `kitchen`                | friendly label shown in the UI (the instance name is the `id`) |
| `codecs`  | `pcm,opus`               | codecs you can decode, preference order                   |
| `rate`    | `48000`                  | max output sample rate                                    |
| `hwvol`   | `1`                      | `1` if you have a hardware volume control                 |
| `delayms` | `30`                     | fixed output-delay you can't change (DAC/amp latency), ms |
| `queue`   | `1`                      | `1` if you can report output-queue depth (drives the servo, §9) |
| `input`   | `0`                      | `1` if you have a line-in capture capability              |

A master (the "room" role) advertises `role=master` plus the existing `gossip`/
`http`/`stream`/`source` ports and its own `name`. A combined node advertises both
sets. **Capabilities never gate membership or fan-out** — they inform the
master/operator (e.g. a UI warning), not the codec choice.

Typical MCU advert: `id=…  role=playback  control=9300  name=kitchen  codecs=opus
rate=48000  hwvol=0  delayms=25  queue=1  input=0`.

---

## 6. Control plane — being driven by a master

A player is a **soft-state** receiver: the master periodically re-asserts the
desired state, and you apply each command **idempotently**. You never need to ack;
the master re-sends on change and on a slow heartbeat (~1 Hz), so a lost control
datagram self-heals. Read all of these on your CONTROL_PORT.

### 6.1 ATTACH (`0x30`) — "join this stream"

Payload (16 bytes, big-endian):

| Offset | Size | Field         | Meaning                                            |
|-------:|-----:|---------------|----------------------------------------------------|
| 0      | 4    | `sourceIP`    | master audio source IPv4                           |
| 4      | 2    | `sourcePort`  | master SOURCE_PORT (HELLO/BYE/RESTART, TCP dial)   |
| 6      | 4    | `clockIP`     | master clock IPv4 (usually == sourceIP)            |
| 10     | 2    | `clockPort`   | master STREAM_PORT (CLOCK_REQ/RSP)                 |
| 12     | 1    | `codec`       | `0`=pcm, `1`=opus                                  |
| 13     | 1    | `transport`   | `0`=udp, `1`=tcp                                   |
| 14     | 2    | `bufferMs`    | playout lead (§9)                                  |

(IPv4 only in v2 — LAN multiroom. IPv6 is a future extension via a new ATTACH type.)

On ATTACH:
- if the `(source,clock,codec,transport)` tuple is **unchanged** from your current
  attachment → it's a heartbeat; just refresh `bufferMs` and continue. **Idempotent.**
- if it **changed** (or you were idle) → (re)point your clock follower at
  `clockEndpoint`, reset your jitter buffer, and **subscribe**: send a HELLO with
  prime-me to `sourceEndpoint` (§9). You do **not** learn `gen` from ATTACH — the
  audio/RECONFIG stream carries it (§9).

### 6.2 DETACH / SETVOL / SETDELAY / SETCAP / SETEQ (master → player)

- **DETACH (`0x31`)** — no payload. Stop playout, drop buffered audio, send a BYE to
  your current source (politeness), go **idle**. Absence of ATTACH heartbeats is also
  idle; DETACH is the explicit, immediate form.
- **SETVOL (`0x32`)** — 2 bytes: `volumePct` (u8, 0–100), `flags` (u8, bit0 = mute).
  Apply a software gain (with a short ramp) or your hardware volume if `hwvol`.
- **SETDELAY (`0x33`)** — 2 bytes: `delayMs` (i16, signed). Output-delay calibration:
  positive = your device chain plays late, so emit each frame earlier. Re-anchors
  playout (drop buffered frames). This is the per-node latency compensation that
  keeps heterogeneous speakers sample-aligned.
- **SETCAP (`0x34`)** — 2 bytes: `capId` (u8), `on` (u8). Runtime enable/disable of a
  capability (e.g. `capId` for line-in). Unknown `capId` → ignore.
- **SETEQ (`0x35`)** — 2 bytes: `delayMs` (u16, unsigned). Cross-room equalization
  delay the master computes: it ADDS this much delay so your room — if it has a
  SMALLER device buffer than the slowest room — falls back into phase with the others.
  SEPARATE from SETDELAY: apply BOTH (your acoustic offset *and* this). Re-anchors
  playout, so apply a changed value only. Always ≥0 (it only ever delays you).

All five are idempotent and periodically re-asserted; applying the same value twice
is a no-op.

### 6.3 STATUS (`0x40`, player → master) + STATUSREQ (`0x41`, master → player)

Send STATUS to the master's `sourceEndpoint` ~1 Hz and on state change. Lets the
master see a starving/drifting room and feed the SPA's per-room health view. The
master never depends on it for correctness — it is observability + adaptation, not
control.

A master may also **poll** you with a **STATUSREQ (`0x41`, no payload)** on your
CONTROL_PORT — reply **immediately** with a STATUS to the address it came from,
**even while idle** (not attached). This is the master's liveness probe for a
non-gossiping player: answering it keeps you alive in the cluster between
attachments; going silent ages you out.

Payload (big-endian):

| Offset | Size | Field          | Meaning                                       |
|-------:|-----:|----------------|-----------------------------------------------|
| 0      | 16   | `nodeID`       | your 16-byte id (correlates to the mDNS advert)|
| 16     | 1    | `flags`        | bit0 synced, bit1 playing, bit2 calibrated|
| 17     | 2    | `buffered`     | jitter-buffer depth, frames                   |
| 19     | 8    | `lastSeq`      | last seq written to the output                |
| 27     | 8    | `offsetNs`     | clock offset estimate (master_ns − local_ns)  |
| 35     | 8    | `rttNs`        | smallest RTT in the clock window              |
| 43     | 4    | `ratePPMx1000` | servo rate correction, ppm×1000 (i32)         |
| 47     | 8    | `played`       | frames written                                |
| 55     | 8    | `silence`      | gap/silence frames inserted                   |
| 63     | 8    | `late`         | frames dropped for arriving past deadline     |
| 71     | 8    | `deviceDelayNs`| measured output (device) latency, ns    |
| 79     | 8    | `phaseErrNs`   | playout phase error vs the smoothed model, ns |

`deviceDelayNs − phaseErrNs` is your servo's calibrated device-queue depth — the
stable per-room constant the master diffs to compute SETEQ. The `calibrated`
flag (bit2) tells the master that value is frozen and safe to use.

---

## 7. Clock sync (required before playout)

Master-anchored, NTP-style, over the master's **clockEndpoint (UDP)**. Run this
continuously once ATTACHed.

**Probe.** Send a CLOCK_REQ (`type=0x10`) to the master's clockEndpoint:
- header: `gen` = your current session gen (echoed back; the master does not filter
  on it), `seq` = a per-probe counter, `pts=0`, `payloadLen=24`;
- payload: 24 bytes = three `i64 BE` `t1|t2|t3` (unused on a request; the master
  overwrites t2/t3). Record your local send time **t1** keyed by `seq`.

**Reply.** The master answers with a CLOCK_RSP (`type=0x11`) echoing your `seq`, with
payload `t1|t2|t3` where **t2** = master receive time, **t3** = master send time.
**Stamp t4 = your local receive time the instant the datagram arrives.** Match by
`seq`; drop unknown/duplicate/late replies and replies whose `gen` ≠ your current gen.

**Per-sample math** (nanoseconds):
```
offset = ((t2 - t1) + (t3 - t4)) / 2      // master_ns - local_ns
rtt    = (t4 - t1) - (t3 - t2)            // >= 0; smaller is better
```

**Estimate.** Keep the **last 30** samples. The offset you use is the **median of the
5 smallest-RTT samples** in that window (best-RTT filtering rejects scheduling
jitter). Until you have ≥ 1 sample you are **unsynced** and **MUST NOT** start playout.

**Cold-start burst.** To start audio quickly while still withholding playout
until synced, probe in a **burst** right after ATTACH — about **10–20 probes over the
first ~200 ms** — then settle to the steady **1 Hz** cadence. (A pure 1 Hz schedule
would cost up to ~1 s just to acquire the first sample, blowing the <500 ms
play-to-sound budget.)

**Conversions:**
```
masterToLocal(t_master) = t_master - offset
localToMaster(t_local)  = t_local  + offset
```

**Monotonic-clock requirement.** t1 and t4 — and every deadline you compare against —
**MUST come from one monotonic clock** (never wall-clock / NTP-stepped time). On the
ESP32 use `esp_timer_get_time()` consistently.

---

## 8. Subscribe + playout flow

Driven by ATTACH (§6.1). The subscribe + playout machinery below is the **v1 data
path, unchanged** — only its *trigger* moved from self-resolution to ATTACH.

### Subscribe (UDP)

1. On ATTACH, take the master's `sourceEndpoint`/`clockEndpoint`.
2. Send a **HELLO with prime-me** (`type=0x20`, payload `[0x01]`) from your audio
   socket to `sourceEndpoint`.
3. The initial HELLO can be lost → **retry up to 3× at 500 ms** while no audio frame
   has arrived, re-requesting prime each time.
4. Once flowing, send a **keepalive HELLO every 5 s** (payload `[0x00]`, no prime).
   The master **expires** any subscriber unseen for **15 s** — so the 5 s keepalive is
   mandatory. (The master's ATTACH heartbeat is separate; you must keepalive the
   *source* yourself.)

### Subscribe (TCP)

Dial `sourceEndpoint` (TCP). Send a HELLO-with-prime as the first length-prefixed
control frame, then read length-prefixed frames (each a full header+payload).
Keepalive HELLO every 5 s on the same connection. No FEC.

### Receiving audio

For each AUDIO frame (`type=0x01`):
- `gen < your session gen` → **drop** (stale generation);
- `gen > your session gen` → a **new/replaced session**: re-arm to that gen (reset
  jitter buffer, set origin on the first new frame);
- buffer the payload keyed by `seq`. Record the `(seq, pts)` of the first frame as the
  session **origin**.

You may ignore FEC entirely; a missing `seq` just becomes a silence frame.

### Playout scheduling

Maintain a small **jitter buffer** (`seq → {pts, payload}`) and a scheduler that emits
**one 20 ms frame per output write, in `seq` order**.

For the next `seq`, compute its master-clock deadline from the session origin (so gaps
still schedule at the right instant):
```
slotPts  = originPts + (seq - originSeq) * 20_000_000      // ns, 20 ms steps
deadline_master = slotPts + bufferMs * 1_000_000           // add the playout lead
deadline_local  = masterToLocal(deadline_master)           // needs clock sync
```

- `bufferMs` (from ATTACH, default **150**) is the playout lead: every member delays
  each frame by `bufferMs` past its pts so late/jittered frames still arrive in time.
  All members and you use the same value → you all emit the same sample at the same
  wall instant → in sync.
- Sleep until `deadline_local` (monotonic clock). Then:
  - frame present → write its payload;
  - frame **missing** (gap) → write a **frame of silence** (keep cadence; don't stall);
  - slot **already a full frame late** (`now > deadline_local + 20 ms`) → **skip it
    instantly without writing** (writing pushes every later frame late forever); count
    and move on.
- Advance to the next `seq`.

**Jitter-buffer sizing.** Hold roughly `bufferMs` worth of frames (`bufferMs/20` ≈
7–10 at the defaults), plus slack; bound it so a burst can't grow it without limit.

### Getting lost / starvation (RESTART)

If **no audio frame for > 2 s**, send a **RESTART with prime-me** (`type=0x22`, payload
`[0x01]`) to your source — "I got lost, re-prime and resume." If audio still doesn't
return (master gone), go idle and await the next ATTACH / mDNS-rediscovery.

### Generation handling on RECONFIG

When a RECONFIG (`type=0x23`) arrives:
- **stop flag set** (bit0): the session ended. Drop buffered audio, go idle, await the
  next session (a new audio stream under a new gen, or a fresh ATTACH).
- **stop flag clear**: settings/gen changed. **Re-arm to the new `gen`** (reset jitter
  buffer, re-establish origin on the first new frame) and **re-subscribe**: send a
  fresh HELLO-with-prime so the master re-primes you under the new generation.

The `gen` is a **per-master** counter; after a master change (a fresh ATTACH to a
different source) reset your floor to 0 and let the first frame / RECONFIG establish
the new master's gen.

---

## 9. Drift correction — the phase-lock servo (Go and MCU)

Playout is **DAC-pull-paced**: your output device's **blocking write** sets the rate
(it accepts the next frame only as fast as the DAC drains), so there is no separate
schedule to correct — the write *is* the clock. But your DAC drains at its own crystal
rate (tens of ppm off), so the master time of the audio actually reaching the speaker
slowly slides off the master clock — and naive correction would skip or pad a whole
20 ms frame (audible). The canonical fix (and what a full ensemble member does) is a
**phase-lock servo + fractional resampler**:

- the controlled variable is your **play head** — the master timestamp of content
  reaching the speaker, `fedPTS − device-queue depth` (how much audio sits between your
  Write and the speaker — ALSA `snd_pcm_delay`, or your I2S/DMA fill). The DAC's true
  rate shows up as this play head drifting ahead of / behind the master clock;
- a **low-pass-filtered proportional** controller holds the **play head on the master
  clock** by trimming the **resample ratio** (input samples consumed per output sample,
  ≈ 1 ± tens of ppm), clamped and slew-limited so there are no audible rate steps.
  Output length is fixed by the DAC pull; the ratio is the only actuator. (A pure
  proportional law — not PI — avoids integral wind-up against the ±10 ms device-queue
  quantization; it trades a tiny, stable standing offset for guaranteed stability. The
  full rationale is in [`architecture/playout-pipeline.md`](../architecture/playout-pipeline.md).)
- a fractional resampler (e.g. 4-tap Catmull-Rom) produces output at that ratio.

**MCU with `queue=1` (§5):** you can observe DMA fill → **run the same loop** (an
S3-class FPU handles Catmull-Rom at 48 kHz stereo). **Without it (`queue=0`):** the
play head is unobservable → hold ratio ≈ 1 and **accept the drift**; the
skip-late/silence-gap behavior of §8 is the **floor**, not the target. This is what
satisfies the inaudible-drift requirement.

---

## 10. Opus specifics (real firmware)

If you decode opus instead of consuming raw PCM:
- the stream is **48 kHz, stereo, 20 ms** opus packets (960 samples/ch/frame); the
  audio payload is one opus packet (not 3840 B). The ATTACH `codec` field tells you.
- decode each packet to s16le before the jitter/playout stage; everything else
  (timing, gen, buffer, servo) is identical.
- **Reset the opus decoder on every generation change** (RECONFIG non-stop, or a jump
  in `gen`): a new session is a new encoder, so carrying decoder state across the
  boundary corrupts the first packets. On a **lost** packet, prefer opus packet-loss
  concealment (decode with a null packet) over hard silence.

---

## 11. Bench / bring-up fallback — fixed & self-directed (NOT a deployment mode)

> **A deployed player is always mDNS-discovered, master-driven, and visible** (§1,
> §5–§6). The modes below **bypass mDNS and master control** — the player becomes
> invisible and loses remote volume/delay/assignment. They exist only for
> **bring-up and the conformance test rig**: the standalone `cmd/player`, and the
> ESP32's opt-in `disc_mode=1`. Never run them as a deployment.

Without a master driving it, a player may skip the control plane and **self-attach**:

- **Fixed:** hard-code the master's source/clock endpoints (`--source <ip:port>
  --clock <ip:port>`) and the codec/transport/bufferMs. Then run §7–§10 directly.
- **Self-directed:** poll `GET http://<master-host>:<httpPort>/api/cluster` every 5 s,
  pick a group (by `--group <id|name>`, else the first with members), find the master
  node, dial-prefer its `observed[*].ip` else its first `addrs` CIDR host, and ATTACH
  yourself to `sourcePort`/`streamPort` with the group's
  `settings.{codec,transport,bufferMs}`.

The subscribe/clock/playout protocol is identical; only the *trigger* and the
visibility differ.

> **Future (not yet implemented): a *visible* mDNS-less bootstrap.** When multicast
> is unavailable (segmented VLANs, mDNS-filtered Wi-Fi), the intended answer is a
> player `--join <master>` that **announces the player to a known master over
> unicast** — after which it is represented and driven exactly like an
> mDNS-discovered player (still visible, still controllable), mirroring the master's
> gossip `--join` seed. This unicast bootstrap is **not in the code today**;
> it is noted here only so the bench modes above are not mistaken for the eventual
> mDNS-less story.

---

## 12. Conformance checklist

A conforming player:

- [ ] Treats `0xE5` as magic+version; **ignores unknown packet types**; drops
      datagrams whose first byte ≠ `0xE5`.
- [ ] Parses the **24-byte big-endian** header exactly (§3).
- [ ] **Announces over mDNS** with the **instance name = node `id`**, `role=playback`,
      its `control` port, `name`, and its capability TXT keys (§5); **never gossips**.
- [ ] Receives master control on its **CONTROL_PORT**; applies ATTACH / DETACH /
      SETVOL / SETDELAY / SETCAP / SETEQ **idempotently** (§6), answers **STATUSREQ**
      (`0x41`) with a STATUS even while idle (§6.3); is **idle until ATTACHed**.
- [ ] On ATTACH, subscribes from **one ephemeral UDP socket** so audio returns to the
      HELLO source addr; uses the master's `sourcePort` for HELLO, `streamPort`/clock
      for probes.
- [ ] Runs the clock follower: `offset = ((t2−t1)+(t3−t4))/2`, median of the **5
      best-RTT of the last 30**, **bursts probes at cold start** then 1 Hz, and
      **withholds playout until synced**.
- [ ] Uses a **single monotonic clock** for t1/t4 and all deadlines.
- [ ] Subscribes with **HELLO+prime**, **retries 3× at 500 ms**, then **keepalives
      every 5 s** (source expiry 15 s).
- [ ] Schedules playout at `deadline = masterToLocal(pts + bufferMs)`, plays
      **silence on gaps**, **skips frames already a full frame late**.
- [ ] Handles **RECONFIG**: stop → idle; non-stop → re-arm to new `gen` + re-HELLO.
      Drops `gen < current`; re-arms up on `gen > current`.
- [ ] (Recommended) Sends **RESTART+prime** after **> 2 s** starvation; emits
      **STATUS** ~1 Hz; runs the **phase-lock servo** when the device exposes a phase
      probe (`Delay()` / output-queue depth).
- [ ] Never gossips, never joins `groups[].members`, never affects codec negotiation.
