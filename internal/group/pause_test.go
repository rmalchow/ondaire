package group

import (
	"errors"
	"testing"
	"time"
)

// TestPauseFreezesAndResumeRearms exercises D39: pause freezes the session
// (state=paused, source stopped, gen alive) and resume bumps the gen, re-anchors,
// and re-arms the source. The local SINK disarm/re-arm now rides the wire-driven
// control plane (the per-node Driver sees state!=playing → DETACH), not the
// engine, so it is not asserted here.
func TestPauseFreezesAndResumeRearms(t *testing.T) {
	self := idN(1)
	r := newRig(self, 1_000_000, false) // effectively endless pull source
	r.cl.setSnap(soloSnap(self))

	if err := r.e.Play("song.wav"); err != nil {
		t.Fatalf("Play: %v", err)
	}
	defer r.e.Close()
	waitFor(t, time.Second, func() bool { return len(r.srv.snapshotReleases()) >= 2 }, "releasing")

	genBefore := func() uint32 { r.e.mu.Lock(); defer r.e.mu.Unlock(); return r.e.gen }()

	// --- pause ---
	if err := r.e.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	pb, _ := r.cl.lastPlayback()
	if pb.pb.State != "paused" {
		t.Fatalf("state = %q, want paused", pb.pb.State)
	}

	// Releases must stop advancing while paused (allow the in-flight tick).
	relAtPause := len(r.srv.snapshotReleases())
	time.Sleep(80 * time.Millisecond)
	relAfter := len(r.srv.snapshotReleases())
	if relAfter > relAtPause+1 {
		t.Fatalf("releases kept advancing while paused: %d -> %d", relAtPause, relAfter)
	}

	// Pause again -> 409 ErrNotPlaying (already paused).
	if err := r.e.Pause(); !errors.Is(err, ErrNotPlaying) {
		t.Fatalf("double Pause err = %v, want ErrNotPlaying", err)
	}

	// --- resume ---
	if err := r.e.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	genAfter := func() uint32 { r.e.mu.Lock(); defer r.e.mu.Unlock(); return r.e.gen }()
	if genAfter <= genBefore {
		t.Fatalf("resume did not bump gen: %d -> %d", genBefore, genAfter)
	}
	pb2, _ := r.cl.lastPlayback()
	if pb2.pb.State != "playing" {
		t.Fatalf("state after resume = %q, want playing", pb2.pb.State)
	}
	// Releases advance again.
	waitFor(t, time.Second, func() bool { return len(r.srv.snapshotReleases()) > relAfter+1 }, "resumed releasing")

	// Resume again -> 409 ErrNotPaused (already playing).
	if err := r.e.Resume(); !errors.Is(err, ErrNotPaused) {
		t.Fatalf("double Resume err = %v, want ErrNotPaused", err)
	}
}

// TestPauseRejectsWhenIdle: pause with nothing playing -> ErrNotPlaying;
// resume with nothing paused -> ErrNotPaused.
func TestPauseResumeRejectWhenIdle(t *testing.T) {
	self := idN(1)
	r := newRig(self, 5, false)
	r.cl.setSnap(soloSnap(self))
	if err := r.e.Pause(); !errors.Is(err, ErrNotPlaying) {
		t.Fatalf("Pause idle = %v, want ErrNotPlaying", err)
	}
	if err := r.e.Resume(); !errors.Is(err, ErrNotPaused) {
		t.Fatalf("Resume idle = %v, want ErrNotPaused", err)
	}
}
