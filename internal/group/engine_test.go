package group

import (
	"context"
	"testing"
	"time"
)

// The engine is a pure PRODUCER: the reconcile goroutine drives no player-side
// wiring. A cluster change while sourcing is absorbed and (eventually) refreshes
// the playback heartbeat; the run must not panic and must keep the session alive.
func TestRunReconcilesOnClusterChange(t *testing.T) {
	self, master := idN(1), idN(2)
	r := newRig(self, 0, false)
	r.cl.setSnap(soloSnap(self))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.e.Run(ctx)

	// A non-sourcing node's reconcile is a no-op for playback: changing the
	// observed cluster must not synthesize a playback write or panic.
	snap := masterSnap(master, defaultSettings(), self)
	for i := range snap.Groups {
		snap.Groups[i].Playback.State = "playing"
	}
	r.cl.setSnap(snap)
	r.cl.signal()

	// Give the reconcile goroutine time to absorb the change.
	time.Sleep(50 * time.Millisecond)
	if _, ok := r.cl.lastPlayback(); ok {
		t.Fatal("non-sourcing node must not write playback on reconcile")
	}
}

// (Removed TestRunHealsOnBoot — self-heal is obsolete under the crosswise model: a
// player whose target is dead simply idles, with no follow reset.)

func TestCloseHaltsSession(t *testing.T) {
	self := idN(1)
	r := newRig(self, 1000, true)
	r.cl.setSnap(soloSnap(self))
	if err := r.e.Play("input:"); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitFor(t, time.Second, func() bool { return len(r.srv.snapshotReleases()) >= 1 }, "first release")

	if err := r.e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if r.srv.stopCount() < 1 {
		t.Fatal("StopSession not called on Close")
	}
}

func TestCloseIdempotent(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	r.cl.setSnap(soloSnap(self))
	if err := r.e.Close(); err != nil {
		t.Fatalf("Close 1: %v", err)
	}
	if err := r.e.Close(); err != nil {
		t.Fatalf("Close 2: %v", err)
	}
}

func TestClosedRejectsOps(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	r.cl.setSnap(soloSnap(self))
	_ = r.e.Close()
	if err := r.e.Play("x"); err != ErrClosed {
		t.Fatalf("Play after close = %v, want ErrClosed", err)
	}
	if err := r.e.Follow(idN(2)); err != ErrClosed {
		t.Fatalf("Follow after close = %v, want ErrClosed", err)
	}
}
