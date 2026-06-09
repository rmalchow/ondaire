package group

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

func TestRunReconcilesOnClusterChange(t *testing.T) {
	self, master := idN(1), idN(2)
	r := newRig(self, 0, false)
	r.cl.setSnap(soloSnap(self))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.e.Run(ctx)

	// Initial reconcile points the CLOCK at self (loopback). The stream
	// subscription is session-gated: an idle group must not HELLO.
	waitFor(t, time.Second, func() bool { return len(r.cc.snapshot()) >= 1 }, "initial clock repoint")
	if subs := r.sub.snapshotSubs(); len(subs) != 0 {
		t.Fatalf("idle group must not subscribe; got %d subs", len(subs))
	}

	// Change to following a master WITH AN ACTIVE SESSION and signal: now the
	// member subscribes.
	r.cl.dialResults[master] = []netip.Addr{netip.AddrFrom4([4]byte{127, 0, 0, 9})}
	snap := masterSnap(master, defaultSettings(), self)
	for i := range snap.Groups {
		snap.Groups[i].Playback.State = "playing"
	}
	r.cl.setSnap(snap)
	r.cl.signal()

	// New model: self's player follows `master` → it subscribes to master's source.
	waitFor(t, time.Second, func() bool {
		subs := r.sub.snapshotSubs()
		return len(subs) > 0 && subs[len(subs)-1].addr.Addr() == netip.AddrFrom4([4]byte{127, 0, 0, 9})
	}, "reconcile on cluster change")
}

// (Removed TestRunHealsOnBoot — self-heal is obsolete under the crosswise model: a
// player whose target is dead simply idles, with no follow reset.)

func TestCloseHaltsSessionAndUnsubscribes(t *testing.T) {
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
	r.sub.mu.Lock()
	unsubs := r.sub.unsubs
	r.sub.mu.Unlock()
	if unsubs < 1 {
		t.Fatal("Unsubscribe not called on Close")
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
