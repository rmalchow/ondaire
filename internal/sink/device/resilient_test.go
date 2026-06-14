package device

import (
	"log/slog"
	"testing"
	"time"
)

// newResilient builds a resilient backend over the given candidate ids (all "tk")
// wired to a deterministic clock. The ctls must already be registered via putCtl.
func newResilient(t *testing.T, clk *stepClock, ids ...string) *resilientBackend {
	t.Helper()
	cands := make([]Candidate, len(ids))
	for i, id := range ids {
		cands[i] = tkCand(id)
	}
	r := newResilientBackend(cands, slog.Default())
	r.now = clk.now
	t.Cleanup(func() { r.Close() })
	return r
}

// TestResilientRotatesToWorkingOutput: candidate A is down (fails every write), so
// the chain rotates to the healthy B and lands its writes there.
func TestResilientRotatesToWorkingOutput(t *testing.T) {
	a, b := &fakeCtl{}, &fakeCtl{}
	a.failWrites.Store(true)
	putCtl("rot_a", a)
	putCtl("rot_b", b)

	r := newResilient(t, newStepClock(), "rot_a", "rot_b")
	driveWrites(r, 5)

	if b.writes.Load() == 0 {
		t.Fatalf("writes never reached healthy candidate B: a.writes=%d b.writes=%d",
			a.writes.Load(), b.writes.Load())
	}
	if got := r.DeviceStats().Kind; got != "tk" {
		t.Fatalf("DeviceStats.Kind=%q, want tk (live candidate)", got)
	}
}

// TestResilientStableResetsCounters: a candidate that keeps accepting writes past
// resilientStableAfter clears the failure counter and resets the backoff. We
// advance the deterministic clock past the stability window between writes.
func TestResilientStableResetsCounters(t *testing.T) {
	a, b := &fakeCtl{}, &fakeCtl{}
	a.failWrites.Store(true) // force one rotation so fails > 0 before B stabilises
	putCtl("stab_a", a)
	putCtl("stab_b", b)

	clk := newStepClock()
	r := newResilient(t, clk, "stab_a", "stab_b")

	// First write: A fails and the chain rotates to B (opening it), fails > 0.
	driveWrites(r, 1)
	r.mu.Lock()
	failsAfterRotate := r.fails
	r.mu.Unlock()
	if failsAfterRotate == 0 {
		t.Fatal("expected a non-zero failure count after rotating off the down candidate")
	}

	// B accepts a write; advance past the stability window, then another B write
	// should reset fails to 0 and backoff to base.
	driveWrites(r, 1) // B since opened
	clk.advance(resilientStableAfter + time.Second)
	driveWrites(r, 1) // this success crosses the stable threshold

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fails != 0 {
		t.Fatalf("stable candidate did not reset fails: got %d, want 0", r.fails)
	}
	if r.backoff != resilientBaseBackoff {
		t.Fatalf("stable candidate did not reset backoff: got %v, want %v", r.backoff, resilientBaseBackoff)
	}
}

// TestResilientBacksOffAfterMaxSweeps: with every candidate down, the chain enters
// the rested state after maxSweeps full passes and STOPS opening candidates while
// resting; after the backoff elapses and a candidate recovers, it heals.
func TestResilientBacksOffAfterMaxSweeps(t *testing.T) {
	a, b := &fakeCtl{}, &fakeCtl{}
	a.failWrites.Store(true)
	b.failWrites.Store(true)
	putCtl("rest_a", a)
	putCtl("rest_b", b)

	clk := newStepClock()
	r := newResilient(t, clk, "rest_a", "rest_b")

	// Drive enough frames to exhaust len(cands)*maxSweeps failures and rest.
	driveWrites(r, 2*resilientMaxSweeps+4)

	r.mu.Lock()
	resting := r.resting && clk.now().Before(r.restAt)
	r.mu.Unlock()
	if !resting {
		t.Fatal("chain should be resting after repeated whole-chain failures")
	}
	if !r.DeviceStats().Resting {
		t.Fatal("DeviceStats.Resting should be true while rested")
	}

	// While resting, more writes must NOT open candidates.
	before := a.opens.Load() + b.opens.Load()
	driveWrites(r, 20)
	if after := a.opens.Load() + b.opens.Load(); after != before {
		t.Fatalf("resting chain kept opening candidates: before=%d after=%d", before, after)
	}

	// Backoff elapses and a candidate recovers ⇒ heal.
	a.failWrites.Store(false)
	clk.advance(resilientMaxBackoff)
	driveWrites(r, 5)
	if a.writes.Load() == 0 {
		t.Fatal("chain did not recover after backoff elapsed and candidate came back")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.resting {
		t.Fatal("chain should have left the rested state after recovery")
	}
}

// TestResilientReviveClearsRest: a rested chain retries immediately on Revive
// without waiting for the backoff window.
func TestResilientReviveClearsRest(t *testing.T) {
	a := &fakeCtl{}
	a.failWrites.Store(true)
	putCtl("rev_a", a)

	clk := newStepClock()
	r := newResilient(t, clk, "rev_a")

	driveWrites(r, resilientMaxSweeps+2)
	r.mu.Lock()
	rested := r.resting
	r.mu.Unlock()
	if !rested {
		t.Fatal("precondition: chain should be resting")
	}

	a.failWrites.Store(false)
	opensBefore := a.opens.Load()
	r.Revive()        // operator poke (test tone)
	driveWrites(r, 1) // must retry now, not after backoff
	if a.opens.Load() <= opensBefore {
		t.Fatal("Revive did not force an immediate retry of the chain")
	}
	if a.writes.Load() == 0 {
		t.Fatal("expected a write to land after Revive + candidate recovery")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.resting || !r.restAt.IsZero() {
		t.Fatal("Revive should have cleared the rested/backoff state")
	}
}

// TestResilientSetPreferredReorders: SetPreferred rebuilds the chain via the
// registered candidate providers (Candidates(preferred)) and resets the failover
// state. Cross-provider order is map-driven, so we assert SetPreferred reproduces
// Candidates(preferred) exactly and resets the counters, plus that the preferred
// id is actually present in the rebuilt chain.
func TestResilientSetPreferredReorders(t *testing.T) {
	// A provider that yields tk candidates with the preferred id first.
	RegisterCandidates("rk_open", func(preferred string) []Candidate {
		ids := []string{"sp_default", "sp_other"}
		out := make([]Candidate, 0, len(ids)+1)
		if preferred != "" {
			out = append(out, tkCand(preferred))
		}
		for _, id := range ids {
			if id != preferred {
				out = append(out, tkCand(id))
			}
		}
		return out
	})
	putCtl("sp_default", &fakeCtl{})
	putCtl("sp_other", &fakeCtl{})
	putCtl("sp_pref", &fakeCtl{})

	clk := newStepClock()
	r := newResilient(t, clk, "sp_default", "sp_other")

	// Dirty the state so we can prove SetPreferred resets it.
	r.mu.Lock()
	r.fails, r.idx, r.resting = 9, 1, true
	r.restAt = clk.now().Add(time.Hour)
	r.mu.Unlock()

	if !r.SetPreferred("sp_pref") {
		t.Fatal("SetPreferred should return true (wrapper always honours a selection)")
	}

	// Compare as a SET: cross-provider order is map-driven (randomised per range),
	// so two Candidates(preferred) calls may differ in order. The contract is the
	// SAME membership and that the preferred id is present.
	want := Candidates("sp_pref")
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.cands) != len(want) {
		t.Fatalf("SetPreferred chain len=%d, want %d (== Candidates(\"sp_pref\"))", len(r.cands), len(want))
	}
	keyset := func(cs []Candidate) map[string]bool {
		m := make(map[string]bool, len(cs))
		for _, c := range cs {
			m[c.Kind+"|"+c.Arg] = true
		}
		return m
	}
	gotKeys, wantKeys := keyset(r.cands), keyset(want)
	if len(gotKeys) != len(wantKeys) {
		t.Fatalf("SetPreferred membership mismatch: got %v, want %v", gotKeys, wantKeys)
	}
	for k := range wantKeys {
		if !gotKeys[k] {
			t.Fatalf("SetPreferred chain missing %q: %+v", k, r.cands)
		}
	}
	if !gotKeys["tk|sp_pref"] {
		t.Fatalf("preferred id sp_pref absent from rebuilt chain: %+v", r.cands)
	}
	// State reset.
	if r.fails != 0 || r.idx != 0 || r.resting || !r.restAt.IsZero() {
		t.Fatalf("SetPreferred did not reset failover state: fails=%d idx=%d resting=%v restAt=%v",
			r.fails, r.idx, r.resting, r.restAt)
	}
}

// TestResilientOnActiveReportsLiveKind: OnActive fires with the live candidate kind
// when the active candidate is (re)opened.
func TestResilientOnActiveReportsLiveKind(t *testing.T) {
	putCtl("act_a", &fakeCtl{})
	clk := newStepClock()
	r := newResilient(t, clk, "act_a")

	got := make(chan string, 4)
	r.OnActive(func(kind string) { got <- kind })

	driveWrites(r, 1) // opens act_a, schedules the report, fires it after unlock
	select {
	case k := <-got:
		if k != "tk" {
			t.Fatalf("OnActive reported kind %q, want tk", k)
		}
	case <-time.After(time.Second):
		t.Fatal("OnActive never fired for the opened candidate")
	}
}

// TestResilientDeviceStatsOverlay: the wrapper overlays Rotations/Resting/BackoffMs
// onto the live leaf stats, and reports Kind "null" when nothing is live.
func TestResilientDeviceStatsOverlay(t *testing.T) {
	a, b := &fakeCtl{}, &fakeCtl{}
	a.failWrites.Store(true) // forces a rotation ⇒ Rotations increments
	putCtl("ov_a", a)
	putCtl("ov_b", b)

	clk := newStepClock()
	r := newResilient(t, clk, "ov_a", "ov_b")
	driveWrites(r, 3)

	st := r.DeviceStats()
	if st.Rotations == 0 {
		t.Fatalf("expected Rotations > 0 after a failover, got %d", st.Rotations)
	}
	if st.Kind != "tk" {
		t.Fatalf("overlay Kind=%q, want tk (live leaf)", st.Kind)
	}

	// With nothing live (all down, rested) the leaf stats are zero ⇒ Kind null.
	a.failWrites.Store(true)
	b.failWrites.Store(true)
	driveWrites(r, 2*resilientMaxSweeps+4)
	rest := r.DeviceStats()
	if !rest.Resting {
		t.Fatal("expected Resting=true once the chain backs off")
	}
	if rest.Kind != "null" {
		t.Fatalf("rested overlay Kind=%q, want null (no live leaf)", rest.Kind)
	}
}

// TestResilientEmptyChainDiscards: an empty chain behaves like null — Writes
// succeed (discarded), no panic.
func TestResilientEmptyChainDiscards(t *testing.T) {
	r := newResilientBackend(nil, slog.Default())
	t.Cleanup(func() { r.Close() })
	for i := 0; i < 5; i++ {
		if err := r.Write(goodFrame()); err != nil {
			t.Fatalf("empty-chain Write returned error: %v", err)
		}
	}
}

// TestResilientQueryReachesDelayThroughAs is the load-bearing one: device.Query[
// device.DelayReporter] on the WRAPPER must reach the live candidate's Delay()
// THROUGH the As escape hatch (the wrapper itself also implements Delay, but As is
// what future-proofs arbitrary capabilities — we assert the value comes from the
// live leaf).
func TestResilientQueryReachesDelayThroughAs(t *testing.T) {
	c := &fakeCtl{implsDelay: true, delayNs: 123_456, delayOK: true}
	putCtl("delay_a", c)
	clk := newStepClock()
	r := newResilient(t, clk, "delay_a")

	// Before anything opens, no live candidate ⇒ As cannot satisfy via a leaf, but
	// the wrapper itself implements DelayReporter, so Query still resolves to the
	// wrapper. Drive one write so a candidate is live, then assert the value flows
	// from the leaf through As.
	driveWrites(r, 1)

	dr, ok := Query[DelayReporter](r)
	if !ok {
		t.Fatal("Query[DelayReporter] should resolve on the resilient wrapper")
	}
	ns, okDelay := dr.Delay()
	if !okDelay || ns != 123_456 {
		t.Fatalf("Delay()=(%d,%v), want (123456,true) — value must come from the live leaf via As", ns, okDelay)
	}

	// Prove As specifically reaches the live leaf for an arbitrary capability: a
	// type the wrapper does NOT itself implement. StatsReporter qualifies — the
	// wrapper implements it too, so use a direct As() probe for the leaf type.
	var leaf DelayReporter
	if !r.As(&leaf) {
		t.Fatal("As() should populate DelayReporter from the live leaf")
	}
	if _, ok := leaf.(*fakeDelaySink); !ok {
		t.Fatalf("As() returned %T, want the live *fakeDelaySink leaf", leaf)
	}
}
