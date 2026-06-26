package cluster

import (
	"path/filepath"
	"testing"
	"time"

	"ensemble/internal/contracts"
	"ensemble/internal/id"
)

func findPreset(s contracts.Snapshot, pid id.ID) (contracts.StreamPresetView, bool) {
	for _, p := range s.StreamPresets {
		if p.ID == pid.String() {
			return p, true
		}
	}
	return contracts.StreamPresetView{}, false
}

func TestStreamPresetCRUDAndSnapshot(t *testing.T) {
	c := newTestCluster(t, id.New(), nil)

	// Create (zero id → fresh id).
	pid := c.SetStreamPreset(id.ID{}, "Radio", "http://host/stream.mp3",
		&contracts.StreamAuth{Scheme: "basic", User: "u", Pass: "secret"})
	if pid.IsZero() {
		t.Fatal("create returned zero id")
	}
	if queued(c) == 0 {
		t.Fatal("create did not broadcast")
	}

	v, ok := findPreset(c.Snapshot(), pid)
	if !ok {
		t.Fatal("preset missing from snapshot")
	}
	if v.Name != "Radio" || v.URL != "http://host/stream.mp3" {
		t.Fatalf("snapshot view = %+v", v)
	}
	if !v.HasAuth || v.AuthScheme != "basic" {
		t.Fatalf("auth hint wrong: %+v", v)
	}

	// The secret is retrievable for play-time resolution but never in the view.
	rec, ok := c.StreamPreset(pid)
	if !ok || rec.Auth == nil || rec.Auth.Pass != "secret" {
		t.Fatalf("stored secret not retrievable: %+v", rec.Auth)
	}

	// Update (same id) bumps version and changes fields.
	c.mu.Lock()
	verBefore := c.doc.StreamPresets[pid].Version
	c.mu.Unlock()
	c.SetStreamPreset(pid, "Radio 2", "http://host/other.mp3", nil)
	c.mu.Lock()
	verAfter := c.doc.StreamPresets[pid].Version
	c.mu.Unlock()
	if verAfter != verBefore+1 {
		t.Fatalf("version not bumped on update: %d -> %d", verBefore, verAfter)
	}
	v, _ = findPreset(c.Snapshot(), pid)
	if v.Name != "Radio 2" || v.HasAuth {
		t.Fatalf("update not applied / auth not cleared: %+v", v)
	}

	// Delete soft-deletes: gone from snapshot and from StreamPreset().
	c.DeleteStreamPreset(pid)
	if _, ok := findPreset(c.Snapshot(), pid); ok {
		t.Fatal("deleted preset still in snapshot")
	}
	if _, ok := c.StreamPreset(pid); ok {
		t.Fatal("deleted preset still resolvable")
	}
}

func TestStreamPresetSecretPreservedOnEdit(t *testing.T) {
	c := newTestCluster(t, id.New(), nil)
	pid := c.SetStreamPreset(id.ID{}, "S", "http://h/s",
		&contracts.StreamAuth{Scheme: "bearer", Token: "tok-123"})

	// Edit keeping the scheme but with a blank secret (write-only UI) → keep token.
	c.SetStreamPreset(pid, "S renamed", "http://h/s2",
		&contracts.StreamAuth{Scheme: "bearer", Token: ""})
	rec, ok := c.StreamPreset(pid)
	if !ok || rec.Auth == nil || rec.Auth.Token != "tok-123" {
		t.Fatalf("token not preserved on blank edit: %+v", rec.Auth)
	}
	if rec.Name != "S renamed" || rec.URL != "http://h/s2" {
		t.Fatalf("edit fields not applied: %+v", rec)
	}

	// Switching to none clears auth entirely.
	c.SetStreamPreset(pid, "S renamed", "http://h/s2", nil)
	rec, _ = c.StreamPreset(pid)
	if rec.Auth != nil {
		t.Fatalf("auth not cleared: %+v", rec.Auth)
	}
}

func TestStreamPresetPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cluster.json")
	self := id.New()

	saved := make(chan struct{}, 4)
	c1, err := New(Config{
		Self:         self,
		GossipPort:   freeUDPPort(t),
		BindAddr:     "127.0.0.1",
		StatePath:    path,
		SaveDebounce: 10 * time.Millisecond,
		saveNotify:   saved,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c1.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	pid := c1.SetStreamPreset(id.ID{}, "Jazz", "http://h/jazz.mp3",
		&contracts.StreamAuth{Scheme: "basic", User: "u", Pass: "p"})
	waitSave(t, saved)
	_ = c1.Close()

	// Fresh cluster, same path, no gossip: preset (incl. secret) must reload.
	c2, err := New(Config{Self: self, GossipPort: 7946, StatePath: path})
	if err != nil {
		t.Fatalf("New2: %v", err)
	}
	defer c2.Close()
	rec, ok := c2.StreamPreset(pid)
	if !ok || rec.Name != "Jazz" || rec.URL != "http://h/jazz.mp3" {
		t.Fatalf("preset not reloaded: %+v ok=%v", rec, ok)
	}
	if rec.Auth == nil || rec.Auth.User != "u" || rec.Auth.Pass != "p" {
		t.Fatalf("auth not reloaded: %+v", rec.Auth)
	}
}
