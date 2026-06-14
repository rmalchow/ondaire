# Other open-source multiroom / network-audio solutions

A survey of systems that don't ship a single self-contained protocol spec we
could archive verbatim, focused on **how they move audio and keep rooms in
sync**. This is background/design reference for ensemble — none of it is part of
the product. Each entry lists what's actually pinned down vs. reverse-engineered,
and a short note on what's relevant to ensemble's own wire protocol.

For the ones that *do* have downloadable specs, see the sibling folders:
`snapcast/`, `slimproto/`, `airplay/`, plus `rtp/` (Roc) and `cast/` (Google Cast).

---

## RTP-family (open standards, no single owner)

The "boring, interoperable" path: don't invent a protocol, compose RFCs. RTP
(RFC 3550) carries timestamped media; RTCP carries sender/receiver reports;
PTP (IEEE 1588) or NTP carries the shared clock; FECFRAME (RFC 6363) adds
loss recovery. Several projects assemble these differently.

### Roc Toolkit  →  see `rtp/roc-network-protocols.md`, `rtp/roc-fec.md`
- C library + CLI for real-time audio over the network. Pure RFC composition:
  RTP **AVPF** profile + **FECFRAME** with two interchangeable FEC schemes —
  **Reed-Solomon** (RFC 6865) and **LDPC-Staircase** (RFC 6816) — plus RTCP XR
  extensions for latency reporting/tuning.
- Receiver runs a **jitter buffer**, **FEC recovery**, and a **frequency
  estimator + resampler** to compensate sender/receiver clock drift. This is
  the same three-part recipe ensemble uses (buffer + recover + rate-servo).
- **Relevance:** the closest published design to ensemble's transport layer.
  Worth reading their FEC scheme trade-offs before tuning our XOR/parity FEC.
  Their choice to make FEC *optional* (to preserve plain-RTP interop) is a
  deliberate interop-vs-robustness lever we also have.

### PulseAudio RTP (`module-rtp-send` / `module-rtp-recv`)
- Sends a null-sink's audio as **RTP over multicast UDP** (e.g. `224.0.0.56`),
  announced via **SAP/SDP**. No FEC, no real clock discipline — receivers free-run
  their own clock, so multi-receiver sync drifts. Fine for one-way "whole-house
  from one box" on a wired LAN; not lip-sync-tight.
- **Relevance:** the canonical example of *what you get without a clock servo* —
  a useful baseline for why ensemble bothers with a master-anchored clock.

### PipeWire RTP / AES67 (`libpipewire-module-rtp-sink` / `-rtp-session`)
- PipeWire's successor modules. Can speak **AES67** (the broadcast-industry RTP
  profile): 48 kHz L24/L16 PCM, fixed packet times, **PTP-disciplined** clock,
  SDP session description. With PTP it's genuinely sample-accurate across devices.
- Community consensus (2025): PipeWire's *native* RTP modules are still rough;
  the robust combo is PulseAudio/PipeWire sender + **GStreamer receiver**, or a
  dedicated AES67 daemon.
- **Relevance:** AES67 shows the pro-audio answer — push the sync problem entirely
  onto PTP and a shared grandmaster clock. ensemble instead carries timing in-band
  so it works on a dumb LAN with no PTP infrastructure.

### AES67 / Dante / Ravenna (pro audio, mostly closed for Dante)
- Studio-grade networked audio. All assume a **PTP grandmaster** on the LAN and
  multicast L24 PCM RTP at tight, fixed latencies. Dante is proprietary
  (Audinate); AES67 and Ravenna are open(ish) interop layers.
- **Relevance:** out of scope (requires managed network + PTP), but the reference
  point for "perfect sync if you control the network."

---

## Snapcast  →  see `snapcast/`
Covered in its own folder. Summary for comparison: **server-pushed** TCP binary
protocol (port 1704), JSON-RPC control (1705); server timestamps PCM/codec chunks,
clients run a buffer + own time-sync handshake (`Time` messages estimate one-way
offset) and resample to a configurable end-to-end latency. Codecs: PCM, FLAC,
Opus, Vorbis. **Closest architectural cousin to ensemble** — centralized source,
dumb-ish synced clients — except ensemble has no fixed server (any node sources)
and discovers peers itself.

## SlimProto / Squeezelite / Lyrion (LMS)  →  see `slimproto/`
Covered in its own folder. Summary: server on TCP **3483**, 4-char-tagged binary
commands (`HELO`, `strm`, `stat`, …); the **server tells the player to open its
own HTTP(S) connection** to fetch the audio stream, then steers playback with
`strm` commands and reads back buffer fullness via `stat`. Sync across players is
server-driven (it nudges each player's output rate from the `stat` reports).
- **Relevance:** the "control plane and data plane are separate" idea — control
  over a small command socket, bulk audio over a separate HTTP fetch — is an
  alternative to ensemble's single framed stream. Note their explicit warning
  that the wiki spec drifts from the Perl source: a caution about doc/impl drift.

## AirPlay / RAOP  →  see `airplay/`
Covered in its own folder. Summary: RTSP/RTP, ~2 s source-specified latency.
AirPlay 2 moves timing to **PTP** via the `nqptp` helper. Closed/reverse-engineered.

---

## Google Cast (Chromecast audio groups)  →  see `cast/gcast-protocol.md`
- **CASTV2**: protobuf messages over **TLS/TCP**; control/discovery is well
  reverse-engineered (see archived `cast/gcast-protocol.md`). Discovery via mDNS.
- **Audio groups**: one device is elected **MultizoneLeader**, others are
  **MultizoneFollower**; followers estimate their clock offset to the leader and
  play each buffer at its timestamp. A fixed playback delay (~500 ms) gives
  followers time to buffer. The actual media path and sync math are **not public**
  (described mainly in Google's patents, e.g. US 10,587,908 / 11,051,066).
- **Relevance:** leader/follower election + timestamped buffers + estimated
  per-follower offset is conceptually very close to ensemble's master-anchored
  clock. Difference: ensemble's election/membership is gossip-driven and serverless.

---

## OwnTone (formerly forked-daapd)
- A *server*, not a new sync protocol: it's a media server that **speaks other
  protocols outward** — RAOP/AirPlay 1, AirPlay 2, Chromecast, and its own
  Roku/RCP-ish outputs — and is controlled via **DAAP/DACP** (Apple Remote) plus
  a modern **JSON REST API** and Websocket events.
- Sync quality is inherited from whatever output protocol it's driving (it relies
  on AirPlay/Cast timing).
- **Relevance:** an integration/aggregation pattern — be a good citizen of
  existing protocols rather than define one. Orthogonal to ensemble's approach.

## Music Assistant
- Like OwnTone, an **orchestrator**: it drives Snapcast, Squeezelite/SlimProto,
  AirPlay, DLNA, Cast, etc. rather than defining a wire protocol. Often paired
  with a Snapcast backend for the actual synced multiroom transport.
- **Relevance:** confirms Snapcast/SlimProto as the de-facto open transports;
  shows demand for "one controller, many speaker protocols."

## GStreamer (network-clock building blocks)
- Not a product, a toolkit, but the cleanest **documented** sync primitive:
  `GstNetClientClock` / `GstNetTimeProvider` (custom NTP-like protocol),
  `GstNtpClock`, and `GstPtpClock` (IEEE 1588). Senders and receivers share one
  pipeline clock; `rtpbin` + `ntp-time-source=clock-time` aligns multiple
  senders/receivers. Slaving methods: **skew** (default — correct when drift
  exceeds a threshold) and **resample** (linear regression on observations).
- **Relevance:** their "slave a local clock to a remote master, then either skew
  or resample" is exactly ensemble's clock-servo problem stated in library terms.
  Good vocabulary and a reference implementation to compare our servo against.
  See also Centricular/Sebastian Dröge's GStreamer-Conf 2015 talk on synchronized
  multi-room playback.

## Sonos (closed, for context only)
- Proprietary. Discovery via **SSDP/UPnP**, control via **UPnP/SOAP** (and a newer
  local API), audio sync over their own scheme on a self-formed mesh (historically
  SonosNet). No spec; listed only to map the commercial baseline ensemble competes
  with on the "it just works, zero-config" axis.

## MPD satellite / Mopidy
- MPD can run in a "satellite" setup (one library server, multiple players) but has
  **no built-in inter-player sync** — people bolt Snapcast on for that. Mopidy is an
  extensible MPD-compatible server, same story.
- **Relevance:** reinforces that "library/playback control" and "synchronized
  multiroom transport" are separable concerns; the open world keeps reaching for
  Snapcast to fill the sync gap.

---

## How these map onto ensemble's design axes

| Axis | Ensemble | Snapcast | SlimProto | AirPlay 2 | Roc | Google Cast | AES67 |
|------|----------|----------|-----------|-----------|-----|-------------|-------|
| Topology | serverless, any node sources | central server | central server (LMS) | source → receivers | sender → receiver | leader/follower group | many↔many |
| Discovery | mDNS + gossip | static / mDNS | static / mDNS (port 3483) | mDNS (Bonjour) | manual / SDP | mDNS | SAP/SDP |
| Control plane | HTTP REST + gossip state | JSON-RPC (TCP/HTTP) | 4-char binary cmds | RTSP | API (in-process) | protobuf/TLS | — |
| Data plane | framed UDP/TCP, 1 stream | TCP binary chunks | separate HTTP fetch | RTP | RTP/AVPF | private | RTP (L24) |
| Clock sync | master-anchored + rate servo | server ts + offset handshake | server nudges player rate | PTP (nqptp) | freq-est + resampler | est. offset to leader | PTP grandmaster |
| Loss recovery | XOR FEC or TCP | TCP (or buffer) | TCP (HTTP) | retransmit/buffer | FECFRAME (RS/LDPC) | private | none (managed net) |
| Codecs | Opus / PCM (+flac/mp3 src) | PCM/FLAC/Opus/Vorbis | PCM/FLAC/MP3/… | ALAC/AAC | PCM/Opus | private | L16/L24 PCM |
| Infra needed | none (dumb LAN) | server host | server host (LMS) | none / PTP | none | Google account/cloud | PTP master |

**Takeaways for ensemble:**
- Everyone solves sync as **shared clock + buffer + rate correction**; the only
  real differences are *where the clock lives* (central server, PTP grandmaster,
  elected leader, or — ensemble — a gossip-elected master) and *whether timing
  rides in-band or out-of-band*. ensemble's in-band, infra-free choice is the
  distinctive bet.
- **FEC is rare** in the consumer multiroom world (most lean on TCP or just a fat
  buffer); Roc is the one to study for doing it properly. Validates ensemble's
  FEC-or-TCP toggle as a differentiator on lossy Wi-Fi.
- The **serverless / zero-config** angle is genuinely uncommon — Snapcast, LMS,
  and OwnTone all need a designated server host; Cast/AirPlay need vendor infra.
  That's ensemble's clearest structural difference.

---

## Sources

- Roc Toolkit — network protocols & FEC: <https://roc-streaming.org/toolkit/docs/internals/network_protocols.html>, <https://roc-streaming.org/toolkit/docs/internals/fec.html>; design articles: <https://gavv.net/articles/new-network-transport/>
- PulseAudio RTP: <https://www.freedesktop.org/wiki/Software/PulseAudio/Documentation/User/Network/RTP/>
- PipeWire RTP/AES67: <https://docs.pipewire.org/page_module_rtp_sink.html>, <https://docs.pipewire.org/page_pulse_module_rtp_send.html>
- Google Cast protocol (reverse-engineered): <https://github.com/dylanmckay/gcast/blob/master/PROTOCOL.md>; multizone patents: US 10,587,908; US 11,051,066
- GStreamer clocks/sync: <https://gstreamer.freedesktop.org/documentation/net/gstnetclientclock.html>, <https://gstreamer.freedesktop.org/documentation/application-development/advanced/clocks.html>
- OwnTone: <https://owntone.github.io/owntone-server/>
- Music Assistant: <https://music-assistant.io/>
- Snapcast / SlimProto / AirPlay: see the dedicated folders in this directory.
