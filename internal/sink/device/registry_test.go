package device

import (
	"log/slog"
	"slices"
	"testing"

	"ensemble/internal/contracts"
)

func TestSplitSpec(t *testing.T) {
	cases := []struct{ spec, name, arg string }{
		{"null", "null", ""},
		{"file:/tmp/x.pcm", "file", "/tmp/x.pcm"},
		{"alsa:hw:0,0", "alsa", "hw:0,0"}, // split on FIRST colon only
		{"  exec  ", "exec", ""},          // trimmed
		{"", "", ""},
		{"k:", "k", ""},
		{":arg", "", "arg"},
	}
	for _, c := range cases {
		name, arg := splitSpec(c.spec)
		if name != c.name || arg != c.arg {
			t.Errorf("splitSpec(%q) = (%q,%q), want (%q,%q)", c.spec, name, arg, c.name, c.arg)
		}
	}
}

func TestBackendNamesSortedAndContainsRegistered(t *testing.T) {
	names := BackendNames()
	if !slices.IsSorted(names) {
		t.Fatalf("BackendNames not sorted: %v", names)
	}
	// The shared init registered these fakes; all must appear.
	for _, want := range []string{"null", "file", "tk", "named_explicit"} {
		if !slices.Contains(names, want) {
			t.Fatalf("BackendNames missing %q: %v", want, names)
		}
	}
}

// TestRegisterAndOpenFactory: a freshly-registered kind opens via openFactory, and
// an unregistered name yields a typed NotRegisteredError.
func TestRegisterAndOpenFactory(t *testing.T) {
	// Fresh name each run (count-safe) with a fresh sawArg the factory writes to —
	// proving openFactory passes the spec arg through to the registered factory.
	var sawArg string
	name := uniqueName("rk_open")
	Register(name, func(arg string, _ *slog.Logger) (Sink, error) {
		sawArg = arg
		return &discardSink{kind: name}, nil
	}, func() bool { return false }) // gated off so it never pollutes HasPlayback

	s, err := openFactory(name, "the-arg", slog.Default())
	if err != nil {
		t.Fatalf("openFactory(%s): %v", name, err)
	}
	defer s.Close()
	if sawArg != "the-arg" {
		t.Fatalf("factory saw arg %q, want %q", sawArg, "the-arg")
	}

	_, err = openFactory("rk_nonexistent", "", slog.Default())
	if err == nil {
		t.Fatal("openFactory of an unregistered kind should error")
	}
	if _, ok := err.(*NotRegisteredError); !ok {
		t.Fatalf("error type = %T, want *NotRegisteredError", err)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	off := func() bool { return false } // gated off so it never pollutes HasPlayback
	name := uniqueName("rk_dup")        // fresh each run so the FIRST register always succeeds
	Register(name, func(string, *slog.Logger) (Sink, error) { return &discardSink{}, nil }, off)
	defer func() {
		if recover() == nil {
			t.Fatal("registering a duplicate name should panic")
		}
	}()
	Register(name, func(string, *slog.Logger) (Sink, error) { return &discardSink{}, nil }, off)
}

// TestAvailableGating: a kind whose available() returns false is registered but not
// "available". rk_real is gated by setAvailable.
func TestAvailableGating(t *testing.T) {
	setAvailable("rk_real", false)
	if available("rk_real") {
		t.Fatal("rk_real should be unavailable when its gate is false")
	}
	setAvailable("rk_real", true)
	if !available("rk_real") {
		t.Fatal("rk_real should be available when its gate is true")
	}
	// nil-available kind ("file") is always available.
	if !available("file") {
		t.Fatal("a kind with nil available() must always be available")
	}
	// Unregistered ⇒ not available.
	if available("rk_nope") {
		t.Fatal("an unregistered kind must not be available")
	}
}

// TestHasPlayback: true iff some non-null/file kind is available. We drive it with
// the gated rk_real and the auto fakes, restoring gates afterward.
func TestHasPlayback(t *testing.T) {
	// Force every gated, non-null/file kind off (tk/named_explicit are opened only
	// via openFactory, so gating them does not affect other tests).
	for _, k := range []string{"rk_real", "alsa", "exec", "tk", "named_explicit"} {
		setAvailable(k, false)
	}
	if HasPlayback() {
		t.Fatal("HasPlayback should be false with every real kind gated off")
	}
	setAvailable("rk_real", true)
	if !HasPlayback() {
		t.Fatal("HasPlayback should be true once a real kind is available")
	}
	setAvailable("rk_real", false) // restore
}

// TestRegisterCandidatesAndDedup: two providers contribute overlapping candidates;
// Candidates assembles them deduped stable by Kind|Arg, preferred-first per provider.
func TestRegisterCandidatesAndDedup(t *testing.T) {
	RegisterCandidates("named_explicit", func(preferred string) []Candidate {
		return []Candidate{
			{Kind: "tk", Arg: "shared", Label: "p1-shared"},
			{Kind: "tk", Arg: "p1-only", Label: "p1-only"},
		}
	})
	RegisterCandidates("rk_real", func(preferred string) []Candidate {
		return []Candidate{
			{Kind: "tk", Arg: "shared", Label: "p2-shared"}, // dup of p1-shared (same Kind|Arg)
			{Kind: "tk", Arg: "p2-only", Label: "p2-only"},
		}
	})

	got := Candidates("pref")
	// Collect the Kind|Arg keys; "shared" must appear exactly once.
	var sharedCount int
	keys := map[string]bool{}
	for _, c := range got {
		k := c.Kind + "|" + c.Arg
		if keys[k] {
			t.Fatalf("Candidates returned a duplicate key %q: %+v", k, got)
		}
		keys[k] = true
		if c.Arg == "shared" {
			sharedCount++
		}
	}
	if sharedCount != 1 {
		t.Fatalf("shared candidate appeared %d times, want 1 (dedup)", sharedCount)
	}
	for _, want := range []string{"tk|shared", "tk|p1-only", "tk|p2-only"} {
		if !keys[want] {
			t.Fatalf("Candidates missing %q: %+v", want, got)
		}
	}
}

// TestRegisterEnumerator: ListOutputDevices aggregates every registered enumerator.
func TestRegisterEnumerator(t *testing.T) {
	RegisterEnumerator("named_explicit", func() []contracts.OutputDevice {
		return []contracts.OutputDevice{{ID: "enum:one", Desc: "first"}}
	})
	devs := ListOutputDevices()
	found := false
	for _, d := range devs {
		if d.ID == "enum:one" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListOutputDevices did not include the registered enumerator output: %+v", devs)
	}
}
