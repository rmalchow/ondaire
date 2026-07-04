# Discovery & cluster

How nodes identify themselves, find each other, share state, and organize into
groups — all with no leader, no consensus, and no external service.

## Node identity

- **Node ID** — 16 random bytes, lowercase hex (32 chars). Generated on first start,
  persisted to `DATA_DIR/node.json`, **immutable** forever after.
- **Node name** — display name, initially the first 8 chars of the node ID. Changeable
  at runtime; persisted and replicated.
- **Volume** — per-node playback gain `0.0–1.0` (default `1.0`), applied continuously
  as software gain in the [sink](playout-pipeline.md). Live; persisted and replicated.
- **Output delay** (`outputDelayMs`) — per-node hardware-latency calibration (default
  0, clamped ±500 ms) for fixed downstream delay the system cannot measure (DAC/amp/
  Bluetooth chains, a player's internal buffer). Subtracted from the playout deadline;
  a change re-anchors playout via a sub-second re-prime. Persisted and replicated.
- **Following** (`following`) — the node's last-known follow target as a 32-hex node id,
  or `""` for a solo master. Persisted so a node that disappears **rejoins its previous
  group on return** (see Groups). The live value is the replicated node-record field;
  this is the boot seed + last-known.
- **Capabilities** — probed at runtime on each start: a `$PATH` scan for exec tools plus
  `dlopen` probes for optional libraries (`libopus.so.0`, `libasound.so.2`, via purego —
  no cgo, no build variants). One universal binary; a host without a library reports
  that capability off:
  - `playback` — a real PCM output backend is available
  - `codecs` — streaming codecs (`["pcm"]`, + `"opus"` when libopus loads)
  - `backends` — sink backends usable here (`alsa` only when libasound loads)
  - `sources` — media-source schemes (`file`, `http`, `input`)
  - `formats` — local media formats it can decode (`wav`, `mp3`, `flac`)

  Reported capabilities are **effective** = probed minus operator-disabled: an operator
  may turn off `playback`, `opus`, or `input` via `PATCH /api/node {disabled}`. The UI
  shows these as tri-state chips (available / unavailable / disabled).

## Ports

Four configurable base ports, each **bind-or-increment**: try the base port; if taken,
increment by 1 and retry (up to 64 attempts). The *actually bound* ports are what the
node advertises.

| Name          | Default | Protocols        | Purpose                                          |
|---------------|---------|------------------|--------------------------------------------------|
| `HTTP_PORT`   | 8080    | TCP              | REST API, WebSocket, SPA, node proxy             |
| `STREAM_PORT` | 9090    | TCP **and** UDP  | member-side stream reception **and** clock sync (UDP, multiplexed by packet type) |
| `SOURCE_PORT` | 9200    | TCP **and** UDP  | audio source: subscriptions + stream control; inbound only matters while a master |
| `GOSSIP_PORT` | 7946    | TCP **and** UDP  | memberlist gossip                                |

TCP and UDP for a given name must end up on the **same** port number: a candidate is
accepted only if *all* its sockets bind, else both close and the next port is tried.
mDNS additionally uses standard multicast UDP 5353. A receive-only player also binds a
**`CONTROL_PORT`** (default 9300) it advertises over mDNS, for master→player commands.

Configuration is via flags with env-var fallbacks (`--http-port` /
`ONDAIRE_HTTP_PORT`, etc.); see [`user/config-reference.md`](../user/config-reference.md)
for the full table.

## Discovery

Two mechanisms, both always on:

1. **mDNS** (`grandcat/zeroconf`): every node registers service `_ondaire._tcp` with
   TXT records (`id`, `role`, the port set, `name`, and — for players — capability keys).
   Every node browses continuously; any peer not yet in the gossip cluster is joined via
   its discovered address. A receive-only player **announces over mDNS but never
   gossips**; its mDNS instance name is its node `id`, stable across reboots, so a master
   always re-discovers the same player under the same key.
2. **Gossip** (`hashicorp/memberlist`): carries liveness *and* the replicated cluster
   state. Once any two nodes have met via mDNS, state spreads to everyone transitively.

### Addresses and observed-IP reporting

Every node lists its own (non-loopback, up) interface addresses in **CIDR notation** in
its node record. But self-reported addresses can be wrong (containers, VPNs,
multi-homing), so nodes also report what they **actually see**: whenever a node receives
gossip or HTTP traffic from a peer, it records the peer's remote IP as an *observed
address* (`peerId → ip, lastSeen`) and publishes its observation map.

When choosing an address to dial a peer, candidates are the peer's self-reported CIDR
list **intersected with the cluster's observations** — an IP no node has ever observed
is ignored — preferring the most recently observed. Two bootstrap exceptions: the
initial gossip join dials the address mDNS answered from, and a peer with no observations
yet falls back to its self-reported list (tightening to observed-only as soon as traffic
flows). The audio path needs no resolution at all: the source streams back to wherever
each subscription came from.

### One player, one driver

A receive-only player has **no HTTP API of its own**; a master drives it over the
control plane, translating every control action (assign, play, volume, …) into
`ATTACH`/`SETVOL`/`DETACH`. A player follows exactly one master — its `following` is
a single replicated record — so only that master's driver acts on it (D62).

**Ownership is gossiped, so multiple masters converge.** When a master sets a
player's `following` (assign / reassign / clear), it **broadcasts** the proxied
record like any other node delta; peers merge it last-writer-wins. So moving a player
from room A to room B is safe: B's assignment carries a higher version, A merges it,
sees the player no longer follows A, and `DETACH`es — exactly one driver at any time.
A discovering master keeps the record *fresh* (mDNS liveness) without bumping its
version, so re-discovery never churns the assignment. (The `UpsertPlaybackNode`
proxy injection stays local — only the **assignment** is gossiped.)

Two narrow caveats remain: a **truly concurrent** assignment of the same player by
two masters within one gossip round (same base version) is resolved by LWW only once
one write lands later — a rare operator race, fixed by re-assigning. And convergence
needs the masters to be in **one gossip cluster** speaking the **same protocol
version**: a stale/old-version master that predates gossiped assignments (or a
partitioned cluster) won't honor it and can still fight over a shared player.

## Replicated cluster state

A single eventually-consistent document, replicated to all gossiping nodes via
memberlist broadcasts + push/pull sync. **No leader, no consensus.** Merging is
**last-writer-wins per record** using a per-record monotonic version (a lamport-ish
counter, tie-broken by node ID).

Three record kinds, with deliberate keying so that membership churn never orphans them:

- **Node records** — owned and only ever written by the node itself: `id, name, volume,
  outputDelayMs, addrs (CIDR), the four ports, capabilities, following, observed
  (peerId → {ip, lastSeen}), version, updatedAt`.
- **Group names (override map)** — `memberXOR → {name, version, updatedAt}`, written by
  whichever node renames the group (LWW). Keyed by the **XOR of the member set** so an
  override names a specific *combination* of rooms and survives master changes and
  re-forming. An empty name clears the override.
- **Playback status** — per group, keyed by the **master's node id** (= the live group
  id), written only by that group's master: `masterId → {state: idle|playing|paused,
  uri, startedAt, positionSec, codec, transport, version, updatedAt}`. `paused` is a
  frozen session: the master keeps the source + session alive but stops releasing
  frames; members treat anything other than `playing` as inactive and unsubscribe.

Liveness (`lastSeen`) comes from memberlist itself; alive/dead is not part of the
document. Node records and playback entries untouched for **30 days** are purged
(checked hourly); **group names and group settings are exempt** — they are a lookup
table kept indefinitely, so a member combination keeps its name whenever it reforms.

### What persists to disk

The group **override-names** map and this node's **own group-settings** record (keyed
by self id, since group id == master id) are persisted to `DATA_DIR/cluster.json` — full
records including version + writer, so the LWW merge applies on reload. Loaded at start
before joining gossip, saved debounced and on clean shutdown. So an offline node still
knows every group name it ever saw, and a master that restarts re-forms its group with
its last codec/transport/bufferMs instead of cluster defaults. Only the node's *own*
settings persist; node records and playback stay runtime/replicated.

## Groups

Groups are **derived, not stored**. The only stored fact is each node's `following`
field — the **player's** target (a node and its player are 1:1, so the field rides the
node record):

- `following == M` (a live master) → the player joins master `M`'s group.
- `following == self` → the player plays its own group.
- `following == ""` → the player is **idle**, in no group. (A dead, unknown, or
  playback-node target is idle too.)

Mastering is **intrinsic**: every alive gossiping (non-playback) node always masters its
own group (group id == node id), even with no players attached — a valid, assignable,
idle zone. `following` only places the *player*; a playback-only node never masters.

**Derivation**, recomputed by every node from the replicated state + liveness: for each
alive gossiping node `M`, `master = M`, `members = {n alive : n.following == M}`. A node
whose `following` points at a dead/unknown/playback/also-following node has its **player
idle**, and additionally **resets its own `following` to ""** after a 10 s grace period
(self-heal). The grace is measured from when the node first *observes* the dangling
follow, not from process start, so slow gossip convergence never insta-clears a follow
that is merely still propagating.

**Group ID = the master's node id.** Keying the group — and its master-written playback
+ settings records — by the master id means membership churn (a member joining/leaving,
master unchanged) never orphans those records: the id only changes on a *master* move,
which is a takeover that stops the session first.

**Group label** — resolved as: an **explicit override** (the member-XOR-keyed name map)
when one exists for the current member set; otherwise a **derived label** computed from
the member names — sorted, joined with `" + "`, capped at the first 3 then `" +N more"`
(e.g. `"bedroom + kitchen + living room +2 more"`). A solo group is just the node's
name. The UI renders derived labels muted/italic.

### Joining ("I follow you")

"Tell alice to join bob" = `POST /api/follow {"target":"<bobId>"}` on alice. Alice
verifies bob is alive and is a master (not following anyone), then sets `following =
bobId` and gossips it. Alice's solo group ceases to exist; bob's group now contains bob
+ alice; **bob stays master**. `POST /api/unfollow` resets to solo. If bob disappears,
every follower reverts to solo via the self-heal rule above.

### Master takeover ("make master")

If a follower wants to play local media, mastership must move to it first. `POST
/api/group/master {"node":"<aliceId>"}` on any group member:

1. The receiving node forwards to the current master (proxy).
2. The master stops any running playback session.
3. The master copies the group's current **settings** record to the new master's key
   (settings are master-keyed, so they would otherwise reset to defaults).
4. The master instructs every member over HTTP to follow alice, and alice to unfollow.

Group membership is unchanged, so the member-set XOR — hence the name override — is
unchanged; only the master moved, so the **group ID changes** to the new master's id and
settings carry over. Members that miss the command follow alice late or self-heal to
solo. The UI exposes this as a **"make master"** button; *playing local media from a
follower does takeover + play in one action.*

### Rejoin on return

Each node persists `following` to `node.json` and re-seeds its record with it at boot,
gossiping it from the start as if set live. So a node that temporarily disappears
(reboot, crash, brief network drop) **rejoins its previous group automatically**: if its
old master is still alive and a master, the same derivation re-forms the group; if the
old master is absent, the dangling follow just triggers the 10 s self-heal grace, the
player goes idle, and the persisted `following` clears to `""`. No special rejoin path —
the existing derivation + self-heal do all of it.

See [sequence diagrams](sequence-diagrams.md) for the follow / takeover / detach
timelines, and [`developer/roadmap-playback-nodes.md`](../developer/roadmap-playback-nodes.md)
for how receive-only players are discovered and assigned.
