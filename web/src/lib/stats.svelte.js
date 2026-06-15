// Polls the master's per-member STATUS telemetry (GET /api/playback/statuses) and
// exposes it as a reactive map keyed by node id. The master collects every member's
// stats from the STATUS control payload, so this surfaces sync health even for
// members with no reachable HTTP API (D56) or on another subnet — unlike a per-node
// status fetch, which the Pis 502.
//
// EXPERIMENTAL (client-side prototype): we also accumulate a rolling per-metric
// history per node (for sparklines) and derive per-minute RATES from the cumulative
// counters (silence/late/inj/drop) — because an ever-growing absolute counter can't
// be coloured ok/warn/danger, but its rate (events/min) can. In production this
// rolling/averaging would live on the master and ship in the STATUS payload; here it
// resets on reload and only spans the session.

import { getPlaybackStatuses } from "./api.js";

export const playbackStats = $state({ byId: {}, hist: {}, at: 0 });

// HIST_LEN samples retained per metric (~ HIST_LEN * intervalMs of history).
const HIST_LEN = 60;

// Continuous gauges: charted directly (already meaningful instantaneously).
//   key → how to read a display value out of the raw STATUS sample.
const GAUGES = {
  offsetMs: (s) => (s.offsetNs ?? 0) / 1e6,
  rttMs: (s) => (s.rttNs ?? 0) / 1e6,
  ratePPM: (s) => s.ratePPM ?? 0,
  phaseUs: (s) => (s.phaseErrNs ?? 0) / 1e3,
  deviceMs: (s) => (s.deviceDelayNs ?? 0) / 1e6,
  buffered: (s) => s.buffered ?? 0,
};

// Cumulative counters → per-minute rates (delta / dt). These are the ones that
// can't be coloured as absolutes; the rate can.
const COUNTERS = {
  silenceRate: (s) => s.silence ?? 0,
  lateRate: (s) => s.late ?? 0,
  injRate: (s) => s.samplesInjected ?? 0,
  dropRate: (s) => s.samplesDropped ?? 0,
};

// module-level (non-reactive) accumulators
const hist = {}; // { [nodeId]: { [metric]: number[] } }
const prev = {}; // { [nodeId]: { ts, raw:{counter:value} } } for rate deltas

function push(nodeId, metric, value) {
  const node = (hist[nodeId] ??= {});
  const arr = (node[metric] ??= []);
  arr.push(value);
  if (arr.length > HIST_LEN) arr.shift();
}

let timer = null;

export function startStatsPolling(intervalMs = 1500) {
  if (timer) return;
  const tick = async () => {
    try {
      const arr = await getPlaybackStatuses();
      const now = Date.now();
      const m = {};
      for (const s of arr || []) {
        m[s.nodeId] = s;

        // continuous gauges
        for (const [metric, read] of Object.entries(GAUGES)) {
          push(s.nodeId, metric, read(s));
        }

        // cumulative counters → events/min, using the previous sample's dt
        const p = prev[s.nodeId];
        const dtMin = p ? Math.max((now - p.ts) / 60000, 1e-6) : 0;
        const raw = {};
        for (const [metric, read] of Object.entries(COUNTERS)) {
          const cur = read(s);
          raw[metric] = cur;
          if (p) {
            const d = cur - (p.raw[metric] ?? cur);
            // a counter reset (e.g. a re-arm) shows as negative — clamp to 0.
            push(s.nodeId, metric, d > 0 ? d / dtMin : 0);
          }
        }
        prev[s.nodeId] = { ts: now, raw };
      }
      playbackStats.byId = m;
      // new top-level ref so $derived consumers re-read the (mutated) arrays.
      playbackStats.hist = { ...hist };
      playbackStats.at = now;
    } catch {
      // non-fatal: stale stats just grey out in the UI
    }
  };
  tick();
  timer = setInterval(tick, intervalMs);
}

export function stopStatsPolling() {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
}
