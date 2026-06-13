# External reference: multiroom & network-audio protocols

Archived documentation for other open-source (and a few closed) multiroom audio
systems, gathered as **design reference** for ensemble's own wire protocol. The
emphasis is on *protocols*: discovery, transport, clock synchronization, loss
recovery, codec negotiation, and control APIs.

**None of this is part of the ensemble product.** It is third-party material,
each under its own upstream license, kept here so we don't have to re-find it and
so design decisions can cite a concrete prior art. Files were fetched on
2026-06-09; upstream may have moved on — every file carries a provenance comment
or a `## Sources` section with the original URL.

For ensemble's *own* protocol, see [`../PLAYER.md`](../PLAYER.md) and
the architecture docs under [`../arch/`](../arch/) (`F-clock.md`, `G-stream.md`,
`D-audio.md`, `E-sink.md`).

## Contents

| Path | What it is | Verbatim? | License |
|------|-----------|-----------|---------|
| `snapcast/binary_protocol.md` | Snapcast wire format (base header + 8 message types) | yes | GPL-3.0 (badaix/snapcast) |
| `snapcast/json_rpc_control.md` | Snapcast JSON-RPC control API | yes | GPL-3.0 |
| `snapcast/json_rpc_stream_plugin.md` | Snapcast stream-plugin RPC | yes | GPL-3.0 |
| `snapcast/configuration.md` | Snapcast stream URIs & server config | yes | GPL-3.0 |
| `slimproto/slimproto-tcp-protocol.md` | SlimProto packet-by-packet spec (Squeezebox/Squeezelite) | yes (HTML→md) | SqueezeboxWiki / Lyrion |
| `slimproto/slimproto-developer-guide.md` | SlimProto developer guide | yes (HTML→md) | SqueezeboxWiki / Lyrion |
| `airplay/shairport-sync-AIRPLAY2.md` | AirPlay 2 architecture (realtime vs buffered, PTP) | yes | shairport-sync (GPL) |
| `airplay/nqptp-ptp-timing.md` | NQPTP — PTP timing helper for AirPlay 2 | yes | nqptp (GPL) |
| `rtp/roc-network-protocols.md` | Roc Toolkit's RFC stack (RTP/AVPF, FECFRAME, RTCP XR) | yes (HTML→md) | Roc Streaming (MPL-2.0) |
| `rtp/roc-fec.md` | Roc FEC schemes — Reed-Solomon & LDPC-Staircase | yes (HTML→md) | Roc Streaming (MPL-2.0) |
| `cast/gcast-protocol.md` | Reverse-engineered Google Cast (CASTV2) protocol | yes | dylanmckay/gcast |
| `other-multiroom.md` | Survey of everything without a single spec to archive: PulseAudio/PipeWire RTP, AES67/Dante, Google Cast multizone, OwnTone, Music Assistant, GStreamer net-clock, Sonos, MPD/Mopidy — plus a comparison matrix vs. ensemble | written | — |

"Verbatim (HTML→md)" means the upstream page was HTML; it was converted with
`html2text` and the site nav/footer chrome trimmed, but the body text is intact.

## The one-paragraph map

Every synced-audio system solves the same core problem — **a shared clock + a
playout buffer + per-receiver rate correction** — and differs mainly in *where
the clock lives* and *whether timing rides in-band or out-of-band*:

- **Snapcast** — central server pushes timestamped chunks over a TCP binary
  protocol; clients run their own offset handshake. The closest cousin to
  ensemble, minus the serverless/gossip part.
- **SlimProto (Squeezelite/LMS)** — separates control (tiny binary command
  socket, port 3483) from data (player fetches audio over its own HTTP);
  server nudges each player's rate from buffer reports.
- **AirPlay / RAOP** — RTSP/RTP with a source-specified ~2 s latency; AirPlay 2
  offloads timing to **PTP** via the `nqptp` helper.
- **Roc** — pure RFC composition (RTP/AVPF + FECFRAME); the reference for doing
  **FEC** and clock-drift resampling properly. Closest to ensemble's transport.
- **Google Cast** — leader/follower groups, timestamped buffers, estimated
  per-follower clock offset; transport itself is private.
- **AES67/PipeWire/PulseAudio RTP** — push sync entirely onto a **PTP
  grandmaster** (AES67) or skip it and free-run (PulseAudio multicast).

ensemble's distinctive bet: **serverless** (any node sources, peers found via
mDNS + gossip), **infra-free** (timing in-band, no PTP grandmaster required),
with a **FEC-or-TCP** toggle for lossy Wi-Fi. The detailed axis-by-axis matrix is
in [`other-multiroom.md`](other-multiroom.md).

## Refreshing

These were pulled with `curl` (markdown) and `curl + html2text` (wiki/Sphinx
pages). Upstream sources, in case they need re-fetching:

- Snapcast docs: <https://github.com/badaix/snapcast/tree/develop/doc>
- SlimProto: <https://wiki.lyrion.org/index.php/SlimProto_TCP_protocol.html>
- AirPlay: <https://github.com/mikebrady/shairport-sync> · <https://github.com/mikebrady/nqptp>
- Roc Toolkit: <https://roc-streaming.org/toolkit/docs/>
- Google Cast: <https://github.com/dylanmckay/gcast/blob/master/PROTOCOL.md>
