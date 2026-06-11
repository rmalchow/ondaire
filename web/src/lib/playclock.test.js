import { describe, it, expect } from "vitest";
import { createPlayClock, reconcile, sample, markSeek } from "./playclock.js";

// helper: play "file:a" at positionSec, anchored at nowMs.
function anchor(c, positionSec, nowMs, uri = "file:a") {
  reconcile(c, { positionSec, uri, playing: true, nowMs }, {});
}

describe("playclock", () => {
  it("advances evenly at 1x between authoritative updates", () => {
    const c = createPlayClock();
    anchor(c, 10, 0);
    expect(sample(c, 0, 0)).toBeCloseTo(10, 5);
    expect(sample(c, 1000, 0)).toBeCloseTo(11, 5);
    expect(sample(c, 2000, 0)).toBeCloseTo(12, 5);
    expect(sample(c, 2500, 0)).toBeCloseTo(12.5, 5);
  });

  it("slews (no stall, no backstep) when a heartbeat lands slightly behind", () => {
    const c = createPlayClock();
    anchor(c, 0, 0);
    // 5 s later we've extrapolated to ~5.0; a slightly-stale beat reports 4.6.
    const before = sample(c, 5000, 0);
    expect(before).toBeCloseTo(5, 3);
    anchor(c, 4.6, 5000); // behind by 0.4 → ignored (we're ahead): no backstep
    const after = sample(c, 5000, 0);
    expect(after).toBeGreaterThanOrEqual(before); // never steps back
    expect(after).toBeCloseTo(before, 3);
    // and it keeps advancing at 1x from there
    expect(sample(c, 6000, 0)).toBeCloseTo(after + 1, 3);
  });

  it("catches up forward (partial slew) when behind the server", () => {
    const c = createPlayClock();
    anchor(c, 0, 0);
    const est = sample(c, 5000, 0); // ~5.0
    anchor(c, 7, 5000, "file:a"); // server says 7, est 5, err +2, < snap(3) → slew
    const after = sample(c, 5000, 0);
    // moved forward by ~SLEW*err = 0.15*2 = 0.3, not the whole 2 (no jump)
    expect(after).toBeGreaterThan(est);
    expect(after).toBeLessThan(est + 1);
  });

  it("snaps on a track change (uri) to the new position", () => {
    const c = createPlayClock();
    anchor(c, 120, 0, "file:a");
    sample(c, 3000, 0); // ~123 on track a
    anchor(c, 0, 3000, "file:b"); // next track starts at 0
    expect(sample(c, 3000, 0)).toBeCloseTo(0, 5);
    expect(sample(c, 4000, 0)).toBeCloseTo(1, 5);
  });

  it("snaps on a large delta (seek)", () => {
    const c = createPlayClock();
    anchor(c, 10, 0);
    sample(c, 1000, 0); // ~11
    anchor(c, 90, 1000); // jumped +79 (> snap) → seek
    expect(sample(c, 1000, 0)).toBeCloseTo(90, 5);
  });

  it("freezes while paused and snaps on resume", () => {
    const c = createPlayClock();
    anchor(c, 30, 0);
    sample(c, 2000, 0); // ~32 playing
    reconcile(c, { positionSec: 32, uri: "file:a", playing: false, nowMs: 2000 });
    expect(sample(c, 2000, 0)).toBeCloseTo(32, 5);
    expect(sample(c, 9000, 0)).toBeCloseTo(32, 5); // frozen: time passes, no advance
    reconcile(c, { positionSec: 32, uri: "file:a", playing: true, nowMs: 9000 });
    expect(sample(c, 9000, 0)).toBeCloseTo(32, 5); // resume snaps to reported
    expect(sample(c, 10000, 0)).toBeCloseTo(33, 5);
  });

  it("ignores stale re-sends (no backward jump from an unrelated cluster push)", () => {
    const c = createPlayClock();
    anchor(c, 40, 0, "file:a"); // last real heartbeat: position 40
    const at3_5 = sample(c, 3500, 0);
    expect(at3_5).toBeCloseTo(43.5, 3); // free-ran ahead of the stale 40
    // an unrelated cluster change re-delivers the SAME values 3.5 s later: must NOT
    // be treated as a seek-back to 40.
    anchor(c, 40, 3500, "file:a");
    const after = sample(c, 3500, 0);
    expect(after).toBeGreaterThanOrEqual(at3_5); // no backward jump
    expect(after).toBeCloseTo(43.5, 3);
    // a genuinely fresh heartbeat (value changed) still reconciles normally.
    anchor(c, 45, 5000, "file:a"); // ~real position at t=5s; est ~45 → small slew
    expect(sample(c, 5000, 0)).toBeCloseTo(45, 1);
  });

  it("markSeek jumps immediately and the server echo is a no-op", () => {
    const c = createPlayClock();
    anchor(c, 10, 0);
    sample(c, 3000, 0); // ~13
    markSeek(c, 90, 3000); // user scrubbed to 90
    expect(sample(c, 3000, 0)).toBeCloseTo(90, 5);
    expect(sample(c, 4000, 0)).toBeCloseTo(91, 5); // free-runs from there
    // the server's echo of the seeked position must not perturb it
    anchor(c, 90, 4000, "file:a");
    expect(sample(c, 4000, 0)).toBeCloseTo(91, 3);
  });

  it("clamps to duration", () => {
    const c = createPlayClock();
    anchor(c, 178, 0);
    expect(sample(c, 5000, 180)).toBeCloseTo(180, 5); // 178+5=183 → clamped
    expect(sample(c, 9000, 180)).toBeCloseTo(180, 5);
  });
});
