package group

import (
	"errors"
	"testing"
	"time"

	"ensemble/internal/contracts"
	"ensemble/internal/stream"
)

func TestSettingsDefaults(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	r.cl.setSnap(soloSnap(self))
	s := r.e.Settings()
	if s.Codec != "opus" || s.Transport != "udp" || s.BufferMs != 300 {
		t.Fatalf("settings = %+v, want opus/udp/300", s)
	}
}

func TestSetSettingsMasterWritesAndReconfigs(t *testing.T) {
	self := idN(1)
	r := newRig(self, 100, true)
	r.cl.setSnap(soloSnap(self))
	// Start a session so the live path engages.
	if err := r.e.Play("input:"); err != nil {
		t.Fatalf("Play: %v", err)
	}
	defer r.e.Close()
	genBefore := r.e.gen
	startsBefore := r.srv.startCount()

	if err := r.e.SetSettings(contracts.GroupSettings{Codec: "pcm", Transport: "tcp", BufferMs: 300}); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}
	sc, ok := r.cl.lastSettings()
	if !ok || sc.s.Transport != "tcp" || sc.s.BufferMs != 300 {
		t.Fatalf("SetGroupSettings = %+v", sc.s)
	}
	if r.e.gen != genBefore+1 {
		t.Fatalf("gen = %d, want %d", r.e.gen, genBefore+1)
	}
	if r.srv.startCount() != startsBefore+1 {
		t.Fatalf("StartSession not re-armed: %d -> %d", startsBefore, r.srv.startCount())
	}
	st, _ := r.srv.lastStart()
	if st.t != stream.TransportTCP || st.bufferMs != 300 {
		t.Fatalf("re-arm start = %+v", st)
	}
}

// New model: settings apply to the group a node masters (its own), so any node may
// set them — even while its player follows another master's group.
func TestSetSettingsAppliesToOwnGroup(t *testing.T) {
	master, self := idN(1), idN(2)
	r := newRig(self, 0, false)
	r.cl.setSnap(masterSnap(master, defaultSettings(), self))
	if err := r.e.SetSettings(contracts.GroupSettings{Codec: "pcm", Transport: "udp", BufferMs: 150}); err != nil {
		t.Fatalf("SetSettings on own group should succeed, got %v", err)
	}
	if sc, ok := r.cl.lastSettings(); !ok || sc.group != self {
		t.Fatalf("settings should be written for the node's OWN group (self), got %+v ok=%v", sc, ok)
	}
}

func TestSetSettingsValidates(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	r.cl.setSnap(soloSnap(self))
	if err := r.e.SetSettings(contracts.GroupSettings{Codec: "bogus", Transport: "udp"}); !errors.Is(err, ErrBadSettings) {
		t.Fatalf("bad codec err = %v, want ErrBadSettings", err)
	}
	if err := r.e.SetSettings(contracts.GroupSettings{Codec: "pcm", Transport: "bogus"}); !errors.Is(err, ErrBadSettings) {
		t.Fatalf("bad transport err = %v, want ErrBadSettings", err)
	}
	if err := r.e.SetSettings(contracts.GroupSettings{Codec: "opus", Transport: "udp"}); !errors.Is(err, ErrNoOpus) {
		t.Fatalf("opus no-cap err = %v, want ErrNoOpus", err)
	}
}

func TestSetSettingsClampsBuffer(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	r.cl.setSnap(soloSnap(self))

	if err := r.e.SetSettings(contracts.GroupSettings{Codec: "pcm", Transport: "udp", BufferMs: 5}); err != nil {
		t.Fatalf("SetSettings low: %v", err)
	}
	sc, _ := r.cl.lastSettings()
	if sc.s.BufferMs != minBufferMs {
		t.Fatalf("low buffer = %d, want %d", sc.s.BufferMs, minBufferMs)
	}

	if err := r.e.SetSettings(contracts.GroupSettings{Codec: "pcm", Transport: "udp", BufferMs: 99999}); err != nil {
		t.Fatalf("SetSettings high: %v", err)
	}
	sc, _ = r.cl.lastSettings()
	if sc.s.BufferMs != maxBufferMs {
		t.Fatalf("high buffer = %d, want %d", sc.s.BufferMs, maxBufferMs)
	}

	if err := r.e.SetSettings(contracts.GroupSettings{Codec: "pcm", Transport: "udp", BufferMs: 0}); err != nil {
		t.Fatalf("SetSettings zero: %v", err)
	}
	sc, _ = r.cl.lastSettings()
	if sc.s.BufferMs != contracts.DefaultBufferMs {
		t.Fatalf("zero buffer = %d, want %d", sc.s.BufferMs, contracts.DefaultBufferMs)
	}
}

func TestSetSettingsIdleWritesRecordOnly(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	r.cl.setSnap(soloSnap(self))
	if err := r.e.SetSettings(contracts.GroupSettings{Codec: "pcm", Transport: "tcp", BufferMs: 150}); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}
	if _, ok := r.cl.lastSettings(); !ok {
		t.Fatal("settings record not written")
	}
	// idle: no StartSession re-arm.
	if r.srv.startCount() != 0 {
		t.Fatalf("StartSession called while idle: %d", r.srv.startCount())
	}
	// gen not bumped while idle.
	if r.e.gen != 0 {
		t.Fatalf("gen = %d, want 0 (idle)", r.e.gen)
	}
}

func TestSetSettingsLiveMidSession(t *testing.T) {
	self := idN(1)
	r := newRig(self, 1000, true)
	r.cl.setSnap(soloSnap(self))
	if err := r.e.Play("input:"); err != nil {
		t.Fatalf("Play: %v", err)
	}
	defer r.e.Close()
	waitFor(t, time.Second, func() bool { return len(r.srv.snapshotReleases()) >= 1 }, "first release")

	genBefore := func() uint32 { r.e.mu.Lock(); defer r.e.mu.Unlock(); return r.e.gen }()
	startsBefore := r.srv.startCount()

	if err := r.e.SetSettings(contracts.GroupSettings{Codec: "pcm", Transport: "tcp", BufferMs: 200}); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}
	// A live settings change bumps the gen and re-arms the source ring under the new
	// gen/transport/buffer; the local player picks it up over the control plane (no
	// in-engine repoint anymore).
	genAfter := func() uint32 { r.e.mu.Lock(); defer r.e.mu.Unlock(); return r.e.gen }()
	if genAfter <= genBefore {
		t.Fatalf("live settings did not bump gen: %d -> %d", genBefore, genAfter)
	}
	if r.srv.startCount() <= startsBefore {
		t.Fatalf("live settings did not re-arm the source: %d -> %d", startsBefore, r.srv.startCount())
	}
	st, ok := r.srv.lastStart()
	if !ok || st.gen != genAfter || st.bufferMs != 200 {
		t.Fatalf("source re-arm = %+v, want gen %d bufferMs 200", st, genAfter)
	}
}
