# Wire protocol

How bytes move between a group's master and its members: the framed packet format,
the packet types, the two transports, forward error correction, and codec
negotiation. This page is the *architectural* view. The exhaustive, byte-level
implementer's reference — everything you need to build a receive-only player from
scratch — is [`developer/player-protocol.md`](../developer/player-protocol.md).

## Canonical audio format

Everything streams as **48 kHz, stereo, s16le** PCM in **20 ms frames** (960
samples/channel, 3840 bytes). Decoders convert to this (mono → duplicated to stereo;
other rates → linear-interpolation resample); the sink always consumes it.

Every frame carries two independent identifiers, because they serve different layers:

- **`seq`** — a 0-based frame counter per session. Ordering, loss detection, and FEC
  block identity.
- **`pts`** — a presentation timestamp in **master-clock nanoseconds**. Playout
  scheduling (see [clock sync](clock-sync.md) and [playout](playout-pipeline.md)).

## The common header

Every packet — audio, FEC, clock, and control — begins with the same fixed 24-byte
header. **All multi-byte fields are big-endian.**

| Offset | Size | Field        | Type   | Meaning                                          |
|-------:|-----:|--------------|--------|--------------------------------------------------|
| 0      | 1    | `magic`      | u8     | always `0xE5`                                    |
| 1      | 1    | `type`       | u8     | packet type (tables below)                       |
| 2      | 4    | `gen`        | u32 BE | session generation; drop audio with `gen < yours`|
| 6      | 8    | `seq`        | u64 BE | frame sequence number, 0-based per session       |
| 14     | 8    | `pts`        | i64 BE | presentation timestamp, master-clock ns          |
| 22     | 2    | `payloadLen` | u16 BE | payload byte count following the header          |
| **24** |      |              |        | **total header size**                            |

`gen` is the **session generation**, bumped on every new play, master change, or
settings change; receivers drop frames from stale generations. On control packets
`gen`/`seq`/`pts` are unused unless stated and set to 0.

## Protocol versioning

**The magic byte `0xE5` IS the version marker.** Every framed packet starts with it,
and two rules make the protocol forward-compatible:

1. Receivers **MUST ignore unknown packet `type` values** — new optional types can be
   added without breaking older receivers.
2. A future *incompatible* revision changes the **magic byte**, not the header layout.
   A receiver that sees a leading byte other than `0xE5` drops the datagram.

So a single leading-byte check is both the framing sanity check and the version gate.
This is exactly what lets a *protocol-minimal* receiver (e.g. ESP32 firmware)
interoperate without tracking the cluster: it implements only the packet types it
needs and drops the rest.

The current revision is **v2**: it adds the control-plane types and the player's mDNS
announcement on top of the v1 data plane (header, audio, FEC, clock,
HELLO/BYE/RESTART/RECONFIG), which is unchanged.

## Packet types

**Data plane** (the master's `SOURCE_PORT` / a member's `STREAM_PORT`):

| Type   | Name      | Direction    | Meaning                                  |
|--------|-----------|--------------|------------------------------------------|
| `0x01` | AUDIO     | src → sub    | header + PCM (3840 B) or Opus payload    |
| `0x02` | FEC       | src → sub    | XOR parity over 4 audio frames (UDP only)|
| `0x10` | CLOCK_REQ | sub → master | clock probe                              |
| `0x11` | CLOCK_RSP | master → sub | clock reply                              |
| `0x20` | HELLO     | sub → src    | subscribe / keepalive; flag: prime-me    |
| `0x21` | BYE       | sub → src    | "leaving, stop sending"                  |
| `0x22` | RESTART   | sub → src    | "I got lost, re-prime and resume"        |
| `0x23` | RECONFIG  | src → sub    | "session/settings changed"; flag: stop   |

**Control plane** (a player's `CONTROL_PORT`; STATUS back to the master's
`SOURCE_PORT`) — used to drive receive-only players:

| Type   | Name      | Direction       | Meaning                                  |
|--------|-----------|-----------------|------------------------------------------|
| `0x30` | ATTACH    | master → player | "join this stream"                       |
| `0x31` | DETACH    | master → player | "leave, go idle"                         |
| `0x32` | SETVOL    | master → player | volume + mute                            |
| `0x33` | SETDELAY  | master → player | output-delay calibration (ms, signed)    |
| `0x34` | SETCAP    | master → player | enable/disable a capability              |
| `0x35` | SETEQ     | master → player | cross-room equalization delay (ms, ≥0)   |
| `0x40` | STATUS    | player → master | telemetry                                |
| `0x41` | STATUSREQ | master → player | liveness poll "send STATUS now"          |

The control plane and the full ATTACH/STATUS payload layouts are documented in
[`developer/player-protocol.md`](../developer/player-protocol.md). The rest of this
page covers the data plane, which is what every full member also speaks.

## Transports — group setting `transport: udp | tcp` (default `udp`)

Members **subscribe** to the master's source (see [media & streaming](media-and-streaming.md));
the source streams back to the address each subscription came from — no address
resolution needed, observed-by-construction.

### UDP (default)

The subscriber HELLOs from its own `STREAM_PORT` UDP socket to the master's
`SOURCE_PORT`; audio then flows source → that observed `addr:port`, **one frame per
datagram**. Packet types multiplex the member's single `STREAM_PORT` UDP socket:
`0x01` audio, `0x02` FEC, `0x10` clock request, `0x11` clock reply.

**XOR FEC.** After every 4 audio frames the source sends one **parity datagram** —
the XOR of the 4 payloads, padded — so any single loss per block of 5 is recovered.
The receiver keeps a small reorder/recovery window. FEC matters because a raw PCM
frame fragments (below); on lossy Wi-Fi, Opus keeps a frame in one datagram so FEC can
actually recover it.

### TCP

The subscriber dials the master's `SOURCE_PORT` over TCP; the persistent connection
carries control frames and **length-prefixed** audio frames master → member (a `u32
BE` byte count, then that many bytes, themselves beginning with the 24-byte header).
No FEC — TCP retransmits. The member-side `STREAM_PORT` TCP listener plays no role in
audio.

## Codec — group setting `codec: pcm | opus` (default `opus`)

- **`opus`** (default): 20 ms Opus at 128 kbps (~320 B/frame), so a stream datagram
  ≈ 344 B stays **under one MTU** and never IP-fragments. libopus is loaded at runtime
  (`dlopen` via purego, `libopus.so.0`); a host where it isn't loadable reports no
  `opus` capability. The **master encodes once** (after decode, before fan-out, one
  encode for all subscribers); **every member decodes** (the sink always consumes
  canonical PCM).
- **`pcm`**: the raw 3840 B frame payload. One datagram is `24 + 3840 = 3864 B`, which
  **IP-fragments into ~3 packets** — losing any fragment drops the whole frame and the
  per-frame XOR FEC cannot recover it. Use pcm only on reliable links.

### Codec negotiation

The master picks the **effective codec** at every session start (play, resume,
settings change): the wanted `settings.codec` **iff every current gossiping member's
effective capabilities include it AND the master can encode it**, else **pcm** (always
universal). So `play` **never fails** for lack of opus — opus is the default, pcm is
the universal fallback. A downgrade is logged; the replicated playback record carries
the *effective* codec. Receive-only players are **invisible to negotiation** — the
group picks its codec from its real (gossiping) members only; a master may *warn* when
assigning a player whose announced codecs don't cover the group codec, but it does not
change the codec for it.

**Mid-session renegotiation.** If a member disables opus, or a non-opus node joins,
while an opus session is playing, the master's reconcile detects that the running
codec is no longer supported by all members and **downgrades the live session to pcm
in place**: bump `gen`, drop the encoder, re-arm the source, broadcast RECONFIG
(members reconnect), resume from the current position. Only downgrades happen
automatically mid-session; an upgrade (opus became newly possible) applies on the next
play/settings change.

## Stream control

A minimal signaling protocol between subscribers and the source, using the same header
framing with the control packet types above. Over UDP, control datagrams go subscriber
→ master's `SOURCE_PORT` (sent from the subscriber's `STREAM_PORT` socket, so the
reply/stream path is observed-by-construction); over TCP they are frames on the
subscription connection.

| Type | Name | Direction | Meaning |
|---|---|---|---|
| `0x20` | HELLO | sub → src | subscribe (or keepalive); flag: prime-me |
| `0x21` | BYE | sub → src | "I am leaving, stop sending" |
| `0x22` | RESTART | sub → src | "I got lost, re-prime and resume" |
| `0x23` | RECONFIG | src → sub | "settings/session changed; re-fetch group settings and resubscribe under the new generation" — also doubles as the stop notice (payload flag) |

HELLO repeats every **5 s** as keepalive; the source expires subscribers unseen for
**15 s**. When group settings change mid-session (codec, transport, bufferMs), the
master bumps the generation, broadcasts RECONFIG, and subscribers reconnect with the
new settings read from the replicated group settings — settings changes therefore
apply **live**, not at next play.

See [media & streaming](media-and-streaming.md) for how the source server prime and
ring buffer use these, and [sequence diagrams](sequence-diagrams.md) for the full
subscribe/play/restart flows.
