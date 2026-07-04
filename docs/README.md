# ondaire documentation

**ondaire** is a self-organizing multiroom audio system. Every node runs the same
single binary; nodes find each other automatically (mDNS + gossip), organize into
**groups**, and play audio in sync. One binary, four ports, no external services, no
database — state is replicated by gossip and everything heals itself.

The docs are organized into four groups. Start with whichever matches what you're
doing.

## 📖 [User guide](user/) — set it up and use it

For people running ondaire. Installation, configuration, the web UI, troubleshooting,
and step-by-step scenarios.

- [User guide home](user/README.md) — the mental model + pick-your-setup
- [Running ondaire](user/running.md) · [Configuration reference](user/config-reference.md)
- [UI reference](user/ui-reference.md) · [Debugging](user/debugging.md) · [Spotify Connect](user/spotify.md)
- Scenarios: [desktop](user/scenarios/desktop.md) · [Raspberry Pi](user/scenarios/raspberry-pi.md) · [NAS / server master](user/scenarios/nas-master.md) · [ESP32 node](user/scenarios/esp32.md)

## 🏗 [Architecture](architecture/) — how it works

The topic-by-topic design reference. Read the [overview](architecture/README.md) for
the model, then dive into a subpage:

- [Discovery & cluster](architecture/discovery-and-cluster.md) — identity, ports, mDNS, gossip, replicated state, groups
- [Wire protocol](architecture/wire-protocol.md) — frame format, packet types, transport, FEC, codec
- [Clock sync](architecture/clock-sync.md) — master-anchored NTP-style follower
- [Media & streaming](architecture/media-and-streaming.md) — sources, source server, prime, restart
- [Playout pipeline](architecture/playout-pipeline.md) — jitter buffer, resampler, phase-lock servo, backends
- [Sequence diagrams](architecture/sequence-diagrams.md) — attach, play, detach flows
- [HTTP API & UI](architecture/api.md) — REST, WebSocket, node proxy, the SPA

## 🔧 [Developer](developer/) — build on it

For people implementing players, firmware, or hardware, and for ongoing engineering work.

- [Player protocol](developer/player-protocol.md) — the self-contained, byte-level spec for a receive-only node (Go or ESP32)
- [ESP32 node](developer/esp32.md) — firmware, hardware, board matrix, provisioning, the web flasher
- [Calibration](developer/calibration.md) — acoustic coherence measurement & calibration
- [Roadmap: playback nodes](developer/roadmap-playback-nodes.md) — the master/playback role-split plan

## 📚 [Reference](reference/) — external prior art

- [External protocols](reference/external/README.md) — archived specs for Snapcast, SlimProto, AirPlay 2, Roc, Google Cast, AES67/RTP, kept as design prior art

---

The repository's top-level [`README.md`](../README.md) is the project's front page;
[`QUICKSTART.md`](../QUICKSTART.md) is the fastest path to a running cluster.
