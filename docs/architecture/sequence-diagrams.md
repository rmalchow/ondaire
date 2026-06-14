# Sequence diagrams

End-to-end timelines for the three flows that tie the subsystems together: **attach**
(a node joins a group), **play** (a master starts streaming), and **detach** (stop,
leave, or get lost). Each step links to the subsystem that owns it.

Conventions: `M` = master, `N` = a full gossiping member node, `P` = a receive-only
[player](../developer/player-protocol.md). Time flows downward.

---

## Attach — joining a group

Every node that plays a group — a full gossiping member *or* a receive-only player —
is driven the **same way**: the group master's [driver](discovery-and-cluster.md)
sends it an `ATTACH` over the control plane, and from there it subscribes to the
master's source and locks to its clock. The only thing that differs is **how the
master learns it should drive the node**: a full member *gossips* its `following`,
while a receive-only player is *assigned* master-side (it never gossips). A master
plays its own speakers by driving its own player over loopback — the same path.

### A full member follows a master

```
operator        N (alice)                         M (bob, master)
   │                                                   │
   │ POST /api/follow {target: bob} ──► N              │
   │                │ following = bob; gossip it ──────┼──► (all nodes re-derive groups)
   │                │                                   │ re-derive: alice ∈ my group
   │                │                                   │ driver: alice exposes a control port
   │  N:CONTROL_PORT ◄─ ATTACH {bob SOURCE_PORT, bob STREAM_PORT, codec, …} ─┤
   │                │ clock follower → bob's STREAM_PORT (burst, then 1 Hz)
   │                │ HELLO+prime ─────────────────────► bob's SOURCE_PORT
   │                │                  ◄──── burst of ring frames (prime) ────│
   │                │ sink primes phase on first future-deadline frame
   │                │                  ◄──── live frames + FEC ───────────────│
   │  N:CONTROL_PORT ── STATUS ~1 Hz ─────────────────► bob SOURCE_PORT
```

- The follow is just a [replicated `following` field](discovery-and-cluster.md#groups);
  every node re-derives the topology from it. Bob stays master.
- The subscribe + [prime](media-and-streaming.md#subscribing-and-priming) is the same
  machinery the master's own sink uses over loopback.
- Playout is withheld until the [clock](clock-sync.md) is synced; the prime lands the
  [phase](playout-pipeline.md#a8-prime), then the servo holds it.

### A master drives a player

```
M (master)                                  P (player)
   │  ◄──── mDNS announce (role=playback, control port, caps) ─── P advertises
   │  inject non-gossiping node record for P; operator assigns it
   │  (P reuses the `following` field, shows up in cluster + UI)
   │                                              │
   │  ATTACH {sourceEP, clockEP, codec,           │
   │          transport, bufferMs} ──► P:CONTROL_PORT
   │                                              │ point clock follower at clockEP
   │                                              │ HELLO+prime ──► sourceEP
   │  ◄──────── burst prime, then live frames ────┤
   │  ◄──────── STATUS ~1 Hz (telemetry) ─────────┤
   │  STATUSREQ (liveness poll) ──► P             │
   │  ◄──────── STATUS (even while idle) ─────────┤
```

- The player is [discovered over mDNS and master-driven](../developer/player-protocol.md);
  it never gossips. ATTACH is **idempotent** and re-asserted ~1 Hz, so a lost control
  datagram self-heals.
- This is now identical to a full member's attach — same `ATTACH` trigger, same data
  path. Only how the master learned to drive it differs: a member gossips `following`;
  a player is assigned master-side. (The member's `following` reaching the master is
  the one extra hop — gossip propagation — vs. an assignment, which is master-local.)

---

## Play — a master starts a session

```
operator     M (master)                         members + own sink (subscribers)
   │ POST /api/play {uri} ──► M
   │             │ pick effective codec from current gossiping members
   │             │ bump gen; open media source for the URI
   │             │ write playback status {playing, uri, codec, …}; gossip ──► (UI updates)
   │             │ start source server on SOURCE_PORT
   │             │
   │             │ release loop (20 ms ticker):
   │             │   ReadFrame → [encode opus] → stamp pts = sessionStart + i·20ms
   │             │   fan out to every live subscriber ──────────────────► AUDIO 0x01
   │             │   every 4 frames (UDP): XOR parity ─────────────────► FEC 0x02
   │             │   append to ring buffer (for late joiners)
   │             │
   │             │ each subscriber: jitter buffer → resampler → gain → device
   │             │   prime on join, then phase-lock servo holds it on the master clock
```

- The [effective codec](wire-protocol.md#codec-negotiation) is chosen at session start
  from the **gossiping** members; `play` never fails for lack of opus (pcm is the
  universal fallback).
- `sessionStart = now + leadMs`; every frame's `pts` is master-clock time, so all
  subscribers — including the master's own player driven over loopback — schedule it
  identically and emit the same sample at the same wall instant.
- A **settings change mid-session** (codec / transport / bufferMs) bumps the gen,
  broadcasts RECONFIG, and subscribers reconnect with the new settings read from the
  replicated group settings — it applies **live**, not at next play. A forced
  [codec downgrade](wire-protocol.md#codec-negotiation) follows the same path.

---

## Detach — stop, leave, and getting lost

### Stop (master ends the session)

```
operator     M (master)                         subscribers
   │ POST /api/stop ──► M
   │             │ bump gen; broadcast RECONFIG (stop flag) ──────────► sub: drop buffer,
   │             │ close media source; clear playback status; gossip      go idle, unsubscribe
```

The natural EOF of a pull-paced (`file`) source does the same thing without an operator
action. `pause` is the soft form: it freezes the session (source + session kept alive,
frames stop releasing); members see `state != playing` and unsubscribe until `resume`.

### Leave (a member unfollows)

```
operator     N (member)                         M (master)
   │ POST /api/unfollow ──► N
   │             │ following = ""; gossip ───────► (all re-derive: N no longer a member)
   │             │ BYE ──► M's SOURCE_PORT (politeness; stop sending)
   │             │ N is now a solo master of its own (idle) group
```

`following = ""` makes N idle; it still masters its own group. The master simply has one
fewer subscriber; the group's member-set XOR changes, but a master move is *not*
involved, so the group id is unchanged.

### Getting lost (the stream dies)

```
N / P (subscriber)                              M (source)
   │ no audio frame for > 2 s (starvation watchdog fires)
   │ RESTART+prime ──────────────────────────────► M
   │                  ◄──── fresh burst prime ─────│   (master still alive)
   │ resume; re-prime phase; carry on
   │
   │ — OR, if the master is gone —
   │ another 2 s of silence: watchdog disarms; unsubscribe locally
   │ group self-heal: following points at a dead master → after 10 s grace,
   │ following clears to ""; player goes idle (master of its own solo group)
```

This is the same [RESTART path](media-and-streaming.md#stop-end-and-getting-lost) whether
the subscriber is a full member or a player. The 10 s [self-heal grace](discovery-and-cluster.md#groups)
is measured from when the node first *observes* the dangling follow, so slow gossip
convergence never insta-clears a follow that is merely still propagating.

### Master change (takeover)

A takeover is a coordinated detach + re-attach: the old master stops the session, copies
group settings to the new master's key, then tells every member (including itself) to
follow the new master. Followers that miss the message follow late or self-heal. Because
membership is unchanged, the name override survives; because the master moved, the group
id becomes the new master's id. See [takeover](discovery-and-cluster.md#master-takeover-make-master).
