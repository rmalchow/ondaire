# Roadmap — master/playback role split

The coding plan for splitting a node into independently-enableable **room**
(`master`) and **player** (`playback`) roles, realizing the v2 master-driven player
described in [player-protocol.md](player-protocol.md).
**Pre-release latitude: we may renumber wire types and reassign ports freely — no
external client to keep compatible.**

## Load-bearing finding

The **data plane is almost unchanged.** A playback node, on `ATTACH`, performs the
*existing* subscribe dance (HELLO+prime → clock-follow → playout). So the source
server ([`internal/source/server.go`](../../internal/source/server.go)) and clock
server ([`internal/clock/clock.go`](../../internal/clock/clock.go)) need **no
structural change**. The new work is three seams:

1. a **control plane** (wire types + a playback-side listener + a master-side driver);
2. **mDNS announcement** of playback nodes + a master path that represents them in
   cluster state *without* gossiping them;
3. a **Player seam** so local and remote playback are driven by one verb set.

## Where things live today

| Concern | Package / file |
|---|---|
| Wire header + types | `internal/stream/wire.go` |
| Source server (subs, prime, RECONFIG, gen) | `internal/source/server.go`, `prime.go`, `ring.go`, `registry.go` |
| Clock server + follower (1 Hz) | `internal/clock/clock.go` |
| Sink (servo, resampler, `SetGain`/`SetDelayOffset`, `DelayReporter`) | `internal/sink/*` |
| mDNS register/browse, TXT, `Peer` | `internal/discovery/discovery.go`, `parse.go` |
| Node records, caps, `DeriveGroups`, `Following`, liveness | `internal/cluster/*` |
| Group engine (drives local sink+stream client, sessions, play) | `internal/group/*` |
| Caps assembly (probe) | wired by main at startup |
| REST/WS, `/api/cluster`, `/api/node` PATCH (volume/delay/device), follow | `internal/api/*` |
| Config (node.json, env), port probing | `internal/config/*`, `internal/netx` |
| Reference receiver | `cmd/player/main.go` |

Today there are **two** playback paths: the full member (group engine → `internal/sink`
+ `internal/stream` client, with servo/resampler/opus) and the standalone player
(PCM-only, no servo). The role split collapses these to **one** playback component
with two front-ends (local in-process, remote over the wire).

## Design choices to confirm before Phase 3

These refine the decisions; recommended defaults in **bold**. Flagged because the user
hasn't ruled on them and they shape Phases 2–3.

1. **Control transport = UDP soft-state.** Master re-asserts desired state (ATTACH +
   SETVOL/SETDELAY) ~1 Hz and on change; playback applies idempotently; no acks. Robust
   to loss, MCU-trivial. (Alt: TCP control channel — more state, no clear win.)
2. **Playback assignment reuses `Following`.** A playback node is a `NodeRecord` with
   `Role=playback`, `Gossips=false`, and `Following=<masterID>` **written by a master/
   operator** (the node never sets its own, since it doesn't gossip). `DeriveGroups`
   already attaches followers to masters — so an assigned playback node shows up as a
   group member with minimal new logic. (Alt: a separate `PlaybackAssignment` map —
   more code, no reuse.)
3. **Liveness for non-gossiping playback nodes** = mDNS browse freshness **OR** recent
   STATUS, expiring when both go stale. (memberlist liveness doesn't apply — they're
   not in it.) `DeriveGroups`/`Snapshot` `isAlive` must consult this for `Role=playback`.
4. **A combined (master+playback) node drives its own playback in-process**, not over
   loopback wire — but through the *identical* `Player` verb interface, so "behaves the
   same" holds. The wire is used only for *remote* playback nodes.
5. **Multi-master convergence** for a discovered playback node reuses the own-record
   version reconcile: the discovering master injects the record; assignment is LWW.

## Phases

### Phase 0 — wire + config scaffolding ✅ DONE
*Landed: control types `0x30`–`0x40` + payload codecs (`stream/control.go`, tested); `config.Role` + `CONTROL_PORT` (`config/role.go`).*

- `internal/stream/wire.go`: add control types `0x30 ATTACH`, `0x31 DETACH`,
  `0x32 SETVOL`, `0x33 SETDELAY`, `0x34 SETCAP`, `0x40 STATUS`; add typed payload
  encode/decode (`AttachPayload`, `StatusPayload`, the 2-byte setters) per
  player-protocol.md §6; update the package comment to v2. **Unit tests** for round-trips.
- `internal/config`: a `Role` set (`master`,`playback`; default **both**), env
  `ENSEMBLE_ROLE`. Reserve/probe a `CONTROL_PORT` (default 9300) via `internal/netx`
  when the playback role is on. Role is runtime config, **not** a replicated field;
  the *advertised* role goes in mDNS TXT.

### Phase 1 — the Player seam (unify local playback) ✅ DONE (standalone wiring deferred → Phase 3)
*Landed: `internal/playback` — `Player` + `localPlayer` (engine refactored to drive it, gate green) and the `Listener` control plane (ATTACH/DETACH/SETVOL/SETDELAY/SETCAP + STATUS, soft-state dedup), tested. Standalone playback-only `cmd/ensemble` bring-up folded into Phase 3 (needs discover+drive to be useful).*

- New `internal/playback`: a `Player` with the verb interface
  `Attach(src,clk netip.AddrPort, codec,transport, bufferMs)`, `Detach()`,
  `SetVolume(pct,mute)`, `SetDelay(ms)`, `SetCap(id,on)`, `Status() StatusPayload`.
  It owns a `clock.Follower`, a `stream` client (UDP/TCP subscribe), and a
  `sink.Sink` — i.e. the existing member playout, extracted behind one interface.
- Refactor the group engine (`internal/group/*`) to drive **this** `Player` for local
  playout instead of wiring sink+stream directly. `localPlayer` = current behavior.
- A **control listener** (CONTROL_PORT UDP reader) translates wire verbs → the same
  `Player` calls. So: playback-only node = control listener + `Player` (no gossip, no
  group engine); master+playback = group engine drives `Player` in-process.
- *Risk:* untangling H's direct sink/stream wiring. Do this behind the interface with
  the existing member e2e green before touching discovery.

### Phase 2 — playback announcement + capabilities (mDNS) ✅ DONE
*Landed: role-aware `discovery` (Peer/Caps/Config, `txtRecords` master-XOR-playback, role-aware `parseEntry`, `Peer.PlaybackOnly()`); `cluster.tryJoin` skips non-gossiping peers; `cmd/ensemble` advertises its role. Tested. The playback-only node actually announcing (binding CONTROL_PORT, real caps) lands with the Phase 3 bring-up.*

- `internal/discovery`: `Config` gains `Role`, `ControlPort`, and a caps struct;
  `txtRecords` emits `role`, `control`, `codecs`, `rate`, `hwvol`, `delayms`, `queue`,
  `input` (player-protocol.md §5). A playback-only node advertises **no `gossip` port**.
- `Peer`/`parse.go`: parse `role`, `control`, and caps; keep gossip/http/stream/source
  for masters. Reuse the capability probe to fill the playback caps.
- `internal/cluster` consume path: on a `Role=playback` peer, **do not join gossip** —
  hand it to the new master-side ingest (Phase 3) instead.

### Phase 3 — master side: discover → represent → assign → drive  🚧 IN PROGRESS
*Done & tested: **Slice A** (cluster represents discovered playback nodes — `NodeRecord`/`NodeView` `PlaybackNode`+`ControlPort`, content-gated `UpsertPlaybackNode`, local-only proxy); **Slice B** (`DeriveGroups` attaches assigned playback nodes, never solo/master; mDNS-freshness liveness; codec-negotiation exclusion in `membersLackingCodec`); **Slice C.1** (STATUS ingest in `source.Server`, `Statuses()`); **C.2** (`playback.remotePlayer` — wire-driving Player); **C.3** (`playback.Driver` — drives assigned playback members of the group it masters: ATTACH+SETVOL+SETDELAY while playing, DETACH idle/unassigned).
Remaining — **Slice C.4 (integration + e2e, needs real binary runs):** ✅ cluster assignment setter `AssignPlaybackNode(node,target)` (write-side, tested). TODO: a master-side API endpoint calling it (assignment is master-local — `handlePatchNode` is self-only, so this is a new route, not a proxied patch); `cmd/ensemble --role playback` bring-up (bind CONTROL_PORT + audio/clock sockets, construct clock follower + stream client + sink + `localPlayer` + `Listener`, playback-only mDNS advert with real caps, NO cluster/source/api/group); wire `playback.Driver` + a control-send socket into the master bring-up in `run()`; end-to-end test (master discovers → assigns → drives a Go playback-only node in sync, verified by running the binary).*

- `internal/cluster`: `NodeRecord` gains `Role`, `Gossips bool`, `ControlPort`, and the
  announced `Caps`. New ingest: a master injects/updates a non-gossiping record for each
  discovered playback peer (own-record reconcile for multi-master convergence). Add the
  playback-liveness source (choice #3) and teach `DeriveGroups`/`nodeView` to use it.
- Assignment: reuse `Following` (choice #2). `PATCH /api/node {following}` on a playback
  id is accepted on a master and written/gossiped as that node's assignment record.
- **Control driver** (master): for each assigned-to-me playback node, a ~1 Hz heartbeat
  goroutine sends `ATTACH` (my source/clock endpoints + group settings) and re-asserts
  `SETVOL`/`SETDELAY` from the node's `volume`/`outputDelayMs` record; `DETACH` when
  unassigned. Lives next to the source server / group engine. Once ATTACHed, the node
  HELLO-subscribes and the **existing** session/RECONFIG/prime machinery covers it
  unchanged.
- Ingest `STATUS` on the source server's UDP control reader (`handleControlUDP`): demux
  `0x40`, correlate by `nodeID`, stash into a runtime status field.

### Phase 4 — clock cold-start burst
- `internal/clock/clock.go`: after `SetMaster` (endpoint change), run a short **burst**
  (~10–20 probes over ~200 ms) before settling to `defaultProbeInterval`. Benefits
  every follower (members too), shrinking play-to-sound toward the <500 ms budget.
  Unit-test the schedule.

### Phase 5 — STATUS telemetry surfaced
- Playback `Player.Status()` from `SinkStats` + `FollowerStats`; emit `0x40` ~1 Hz / on
  change to the master source endpoint.
- Master: `NodeView` gains a `PlaybackStatus` sub-struct (buffered, lastSeq, offsetNs,
  rttNs, ratePPM, played/silence/late). Surfaced in `/api/cluster` + WS.

### Phase 6 — SPA
- Show discovered playback nodes (role/caps badges); assign/unassign to a group (writes
  `following`); per-room health from STATUS. Per-node volume/delay controls already
  exist (`PATCH /api/node`) — reuse; they now reach MCU nodes via the master's control
  driver.

**Design constraints (Dieter Rams review of the current SPA — hold the zero-config
line, "weniger aber besser"):**
- **One action to be useful.** A discovered playback node must require exactly *assign
  it to a group* — no settings panel, no wizard. **Reuse the existing mechanism**: the
  "Add node…" dropdown in `GroupCard.svelte:235` is fed by `derive.js:106`
  (`addTargets` = alive nodes in no group); a freshly-discovered playback node surfaces
  there automatically. Do **not** add a new assignment surface.
- **Per-room health = traffic lights, not a panel.** Reuse the `restarts`-style pattern:
  a healthy member row shows **nothing**; a degraded one (drift, low buffer, unsynced)
  shows a small amber marker + value next to the member name (`MemberRow.svelte:18–50`
  has the spacer for it). No tables, no health column, no diagnostics view. Green = silent.
- **Output-delay stays behind the existing calibration `<details>`** (`GroupCard.svelte`
  advanced) — calibration sets it automatically; do not add a per-node delay slider
  to the member row.
- **Playback-only / ESP32 cards omit the feature + formats chip rows** when nothing is
  toggleable/meaningful (`NodeRow.svelte:147–172`) — a node with `playback:true`, no
  formats, no input should not show three struck-through chips.
- **Caps inform, never gate**: a pcm-only speaker assigned to an opus group may get
  a quiet inline warning, but assignment is never blocked.

**Separate, independent UI cleanups Rams flagged (not role-split-specific; do when
convenient, see his prioritized list):** drop redundant header status text
(`App.svelte:89`); rename "Nodes"→"Rooms" + replace the ⚙ glyph (`App.svelte:93`,
`Nodes.svelte:16`); suppress "0 listeners / 0 reconnects" unless `restarts>0` or members
missing (`MemberRow.svelte:35`); hide the Media node-picker when a group is selected
(`Media.svelte:121`); move IP/CIDR/port lists into a `<details>` (`NodeRow.svelte:137–145`);
translate calibration confidence float → good/marginal/failed (`Calibrate.svelte:64`);
demote per-member volume sliders under the group slider (`GroupCard.svelte`).

### Phase 7 — reference client, tests, arch docs
- `cmd/player`: add the **driven** mode (mDNS announce + CONTROL_PORT listener +
  ATTACH-driven subscribe); keep `--source/--clock` fixed and `--node` self-directed as
  the bench/bring-up fallback (player-protocol.md §11). Stays stdlib-only.
- e2e (K): master + Go playback-only node — discover, assign, sync, play to null sink;
  assert drift bounded, cold-start < 500 ms, volume/delay applied over the wire, STATUS
  flowing.
- Update the affected [architecture docs](../architecture/) (discovery-and-cluster,
  wire-protocol, clock-sync, media-and-streaming, playout-pipeline, api) to the new
  model; player-protocol.md is already revised.

## Sequencing
- **0 → 1** first (wire + Player seam), keeping the existing member e2e green.
- **4** (clock burst) is independent — can land any time.
- **2 → 3 → 5 → 6** in order; **7** last.
- **Smallest proof:** Phase 0 + 1 + a hand-rolled ATTACH sender driving a fixed-mode
  playback node — validates the control plane before building discovery/assignment.

## Risks
- **Player extraction (Phase 1)** is the highest-risk refactor; gate it on the member
  e2e staying green.
- **Liveness/assignment races** across masters — covered by choices #3/#5.
- **IPv4-only ATTACH** in v2 — note as a limitation; IPv6 is a future ATTACH type.
- **Unauthenticated control plane** on the LAN — same trust model as today's gossip/
  source ports; revisit with the auth/HA work, not here.
