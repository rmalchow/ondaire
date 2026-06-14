# Clock sync

Every member schedules playout against a single shared **master clock**. The master
anchors it; every member — **including the master itself, against localhost** — runs an
NTP-style follower that learns the offset between its own monotonic clock and the
master's. This is what makes `pts` (master-clock nanoseconds) meaningful on every node.

It runs over **`STREAM_PORT` UDP**, multiplexed with stream reception by packet type
(`0x10` request, `0x11` reply). See the [wire protocol](wire-protocol.md).

## The exchange

- The **master** answers clock requests: it echoes the client's `t1`, stamps its
  receive time `t2` and send time `t3` (all monotonic-derived nanoseconds).
- Every **follower** sends a `CLOCK_REQ` and stamps `t4` the instant the reply
  arrives. From one round trip:

```
offset = ((t2 − t1) + (t3 − t4)) / 2      // master_ns − local_ns
rtt    = (t4 − t1) − (t3 − t2)            // ≥ 0; smaller is better
```

## The estimate

The follower keeps the **last 30 samples** and uses the **median offset of the 5
smallest-RTT samples** in that window. Best-RTT filtering rejects scheduling jitter:
the lowest-RTT exchanges are the ones least perturbed by queueing, so their offset
estimate is the cleanest. Until at least one sample has arrived the member is
**unsynced** and **must not start playout** — `MasterToLocal`/`LocalToMaster` report
`ok=false`.

Conversions, once synced:

```
masterToLocal(t_master) = t_master − offset
localToMaster(t_local)  = t_local  + offset
```

## Cadence and cold start

Steady state is **one probe per second**. But a pure 1 Hz schedule would cost up to
~1 s just to acquire the first sample, blowing the play-to-sound budget. So a follower
**bursts** right after it starts — about **10–20 probes over the first ~200 ms** — to
acquire a confident offset quickly, then settles to the steady 1 Hz cadence. Audio is
still withheld until synced; the burst just shortens the wait.

## Monotonic clock requirement

`t1` and `t4` — and every deadline a node compares against — **must come from one
monotonic clock**, never wall-clock / NTP-stepped time (which can jump backwards). On
Linux that is Go's monotonic `time` source (`clock.MonoNow()`); on an ESP32 it is
`esp_timer_get_time()` used consistently.

## When mastership changes

The master clock is anchored by whoever is master. When mastership moves (a takeover,
see [discovery & cluster](discovery-and-cluster.md)), the session generation is bumped
and followers **discard their samples and resync** against the new master. The `gen`
on the clock header rides along so stale replies are dropped. A follower that has not
yet reacquired is unsynced and withholds playout until it has a fresh offset — so a
master change produces a brief, clean re-prime rather than a glitchy hand-off.

## Why this is enough

The acoustic accuracy the system actually achieves is finer than the network clock
sync — the clock offset is RTT-floored at tens-to-hundreds of µs on Wi-Fi, while the
at-the-speaker alignment is held to sub-millisecond by the [playout
servo](playout-pipeline.md) and per-node calibration. The clock's job is only to get
every node onto the same timeline to within the buffer lead; the servo does the fine
phase work from there. This is also why the clock sync can be independently *validated*
by acoustic measurement — see [`developer/calibration.md`](../developer/calibration.md).
