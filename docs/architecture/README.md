# Architecture overview

Ensemble is a self-organizing multiroom audio system. Every node runs the **same
single binary**. Nodes find each other automatically (mDNS + gossip), organize
into **groups**, and play audio in sync: the group **master** runs an audio
**source** that decodes media into timestamped PCM (or Opus); every member —
including the master itself — **subscribes** to that source and plays the stream
out through a clock-disciplined **sink**. A master-anchored clock plus a per-node
phase-lock servo keeps every speaker aligned.

The guiding constraint is **simple and basic**: one binary, four ports, no
external services, no database, no PKI. State is replicated by gossip; everything
heals itself.

This section is the topic-by-topic architecture reference. Read this page for the
model, then dive into the subpage for the part you care about:

| Subpage | What it covers |
|---|---|
| [Discovery & cluster](discovery-and-cluster.md) | node identity, ports, mDNS, gossip, the replicated document, groups (follow / takeover / self-heal) |
| [Wire protocol](wire-protocol.md) | the framed packet format, packet types, UDP/TCP transport, FEC, codec negotiation, protocol versioning |
| [Clock sync](clock-sync.md) | master-anchored NTP-style clock follower |
| [Media & streaming](media-and-streaming.md) | media sources, the audio source server, subscription, ring buffer, prime, restart |
| [Playout pipeline](playout-pipeline.md) | the sink: jitter buffer, fractional resampler, phase-lock servo, output backends |
| [Sequence diagrams](sequence-diagrams.md) | end-to-end flows: attach (join a group), play, detach (stop / leave / get lost) |
| [HTTP API & UI](api.md) | REST, WebSocket, the node proxy, the Svelte SPA |

For building a **player** (a receive-only node — Go daemon or ESP32 firmware), the
self-contained byte-level spec is [`developer/player-protocol.md`](../developer/player-protocol.md).

---

## The node model

A node is one process. It is identified by a **16-byte node ID** (32 hex chars),
generated on first start and immutable forever after. Everything else about a node
— name, volume, calibration, group membership — is mutable runtime state.

A node runs up to two independently-enableable **roles**:

- **room** (the `master` role) — gossips, owns its slice of the cluster state,
  serves the HTTP API + web UI, sources audio, and *drives* players.
- **player** (the `playback` role) — a receive-only participant that plays a
  group's audio in sync. It never gossips and holds no cluster state; a master
  discovers it over mDNS and drives it.

A node can be a room, a player, or both. "room" / "player" are the user-facing
names; the wire and CLI keep the original `master` / `playback` role names.

Every alive gossiping node always **masters its own group** (group ID == node ID),
even with no players attached. Group membership is *derived*, not stored — see
[Discovery & cluster](discovery-and-cluster.md).

## How the pieces fit

```
                       ┌──────────────────────────── one node ────────────────────────────┐
   mDNS  ──discover──► │  discovery ─► cluster (gossip, replicated LWW state) ─► group     │
                       │                                          engine (follow/takeover) │
   gossip ◄──state───► │                                                │                  │
                       │                                          play  ▼                  │
                       │   media source ─► source server ──fan-out──► subscribers          │
                       │   (file/http/input)   (ring buffer,         (every member,        │
                       │                        burst prime)          incl. this node)     │
                       │                                                │                  │
   clock  ◄──sync────► │   clock follower ──────────────────────────►  sink (jitter buf,   │
   (UDP)               │                                                resampler, servo,  │
   audio  ◄──stream──► │                                                device backend)    │
                       │   HTTP API + WebSocket + node proxy + Svelte SPA                   │
                       └───────────────────────────────────────────────────────────────────┘
```

1. **Discovery** registers the node over mDNS and browses for peers; any peer not
   yet in the gossip cluster is joined.
2. **Cluster** is the memberlist gossip layer plus the single eventually-consistent
   replicated document (node records, group names, playback status). It also
   derives the live group topology from that document plus liveness.
3. **Group engine** acts on the derived topology: it handles follow/unfollow,
   master takeover, self-heal, and — when this node is a master serving a group —
   orchestrates playback sessions.
4. On **play**, the master opens a **media source** for the URI and runs a **source
   server**: it stamps each 20 ms frame with a master-clock presentation timestamp
   and fans it out to every subscriber.
5. Every member — *including the master, over loopback* — runs a **clock follower**
   and a **sink**: jitter buffer → resampler → gain → output device. The sink holds
   each speaker on the master clock.
6. The **HTTP API**, WebSocket, node proxy and **SPA** are the control surface.

## The two timing problems

Multiroom sync is two separate problems, and the design keeps them apart:

- **Phase** — *when* a chunk of audio reaches the speaker (cross-room alignment).
  Solved by stamping every frame with a master-clock PTS, sharing one master clock
  via the [clock follower](clock-sync.md), and scheduling playout against it.
- **Rate** — *how fast* audio is fed to the DAC. Each node's crystal runs at its
  own ~48 kHz (tens of ppm off), so left alone two speakers drift apart. Solved in
  the [playout pipeline](playout-pipeline.md): the DAC's blocking write paces the
  rate, and a phase-lock servo trims a fractional resampler to hold the play head
  on the master clock.

## Canonical audio format

Everything streams as **48 kHz, stereo, s16le** PCM in **20 ms frames** (960
samples/channel, 3840 bytes). Decoders convert to this; the sink always consumes
it. Each frame on the wire carries both a **`seq`** (frame counter — ordering, loss
detection, FEC block identity) and a **`pts`** (presentation timestamp in
master-clock nanoseconds — playout scheduling). See the [wire protocol](wire-protocol.md).

## Design philosophy

- **One universal binary.** Optional native libraries (libopus, libasound) are
  loaded at runtime via `dlopen` (purego) — no cgo, no build variants. A host
  without a library simply reports that capability off.
- **No leader, no consensus.** State is one document, merged last-writer-wins per
  record. Groups are derived from it; failover is just re-derivation after a node
  drops out of gossip.
- **Observed-by-construction networking.** Nodes prefer addresses they have
  actually seen traffic from; the audio path needs no address resolution at all —
  the source streams back to wherever each subscription came from.
- **Self-healing everywhere.** A node that disappears and returns rejoins its old
  group automatically; a subscriber that loses the stream re-primes; a wedged
  output device fails over to the next.

## Repository layout

```
cmd/ensemble/        main: flag parsing, wiring, lifecycle
cmd/player/          standalone reference player (proves player-protocol.md)
cmd/soundcheck/      local tone/bring-up tool
internal/id/         node/group IDs (gen, parse, XOR)
internal/config/     flags/env, data dir, node.json persistence
internal/dl/         runtime shared-library loading (purego dlopen/dlsym)
internal/netx/       bind-or-increment listeners, CIDR interface scan
internal/discovery/  mDNS register + browse
internal/cluster/    memberlist wrapper, replicated state (LWW), observed IPs
internal/group/      group derivation, follow/takeover, playback orchestration
internal/clock/      clock server + follower (UDP, via stream mux)
internal/source/     audio source server: subscriber registry, ring buffer, prime
internal/stream/     frame wire codec, member-side UDP mux, subscriber client
internal/audio/      media sources, decoders, resampler, opus (runtime libopus)
internal/playback/   the player role: player seam + master-side driver
internal/sink/       playout engine; sink/device/ holds the device port + adapters
internal/api/        Echo server: REST, WebSocket, proxy, SPA embed
web/                 Svelte SPA (vite project; npm run build → web/dist)
docs/                this documentation set
```

## Out of scope (v1)

Seek, auth/TLS, internet-facing operation, playlists, album art/metadata, multiple
simultaneous streams per group, hardware mixer volume (volume is software gain in
v1). Per-group play/pause **is** supported.
