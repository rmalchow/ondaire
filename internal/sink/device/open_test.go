package device

import (
	"path/filepath"
	"testing"
)

// resetAutoGates forces the availability-gated auto fakes into a known state.
func resetAutoGates(alsaOK, execOK bool) {
	setAvailable("alsa", alsaOK)
	setAvailable("exec", execOK)
}

// patchAuto rewires the open.go auto chain (which hard-codes "alsa"/"exec") onto
// the test fakes for the duration of one test, restoring on cleanup. open.go calls
// available("alsa")/openFactory("alsa",..) etc.; in the device test binary those
// real kinds are absent, so we register thin "alsa"/"exec" aliases that delegate to
// the auto fakes. They are registered ONCE here, guarded so re-entry is a no-op.
func TestOpenNullBypassesChain(t *testing.T) {
	b, name, err := Open("null", nil)
	if err != nil || name != "null" {
		t.Fatalf("Open(null): name=%q err=%v", name, err)
	}
	defer b.Close()
	if _, ok := b.(*discardSink); !ok {
		t.Fatalf("Open(null) returned %T, want the null discard sink", b)
	}
}

func TestOpenFileResolvesPathArg(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.pcm")
	b, name, err := Open("file:"+path, nil)
	if err != nil || name != "file" {
		t.Fatalf("Open(file:...): name=%q err=%v", name, err)
	}
	defer b.Close()
	// Empty file path errors.
	if _, _, err := Open("file:", nil); err == nil {
		t.Fatal("Open(file:) with empty path should error")
	}
}

func TestOpenExplicitNamedKind(t *testing.T) {
	b, name, err := Open("named_explicit", nil)
	if err != nil || name != "named_explicit" {
		t.Fatalf("Open(named_explicit): name=%q err=%v", name, err)
	}
	defer b.Close()
	if _, ok := b.(*discardSink); !ok {
		t.Fatalf("got %T", b)
	}
}

func TestOpenUnknownErrors(t *testing.T) {
	if _, _, err := Open("definitely_not_registered", nil); err == nil {
		t.Fatal("Open of an unknown backend should error")
	}
	if _, _, err := Open("file:", nil); err == nil {
		// empty path is a different error path; just confirm error.
		t.Fatal("Open(file:) should error")
	}
}

// TestOpenAutoPrefersAlsaThenExecThenNull walks the openAuto preference ladder via
// the availability gates on the fake "alsa"/"exec" kinds.
func TestOpenAutoPrefersAlsaThenExecThenNull(t *testing.T) {
	// alsa available ⇒ auto picks alsa.
	resetAutoGates(true, true)
	if _, name, err := OpenDevice("auto", "", nil); err != nil || name != "alsa" {
		t.Fatalf("auto with alsa available: name=%q err=%v, want alsa", name, err)
	}
	// alsa down, exec up ⇒ auto picks exec.
	resetAutoGates(false, true)
	if _, name, err := OpenDevice("auto", "", nil); err != nil || name != "exec" {
		t.Fatalf("auto with only exec: name=%q err=%v, want exec", name, err)
	}
	// both down ⇒ auto degrades to null, never errors.
	resetAutoGates(false, false)
	if _, name, err := OpenDevice("", "", nil); err != nil || name != "null" {
		t.Fatalf("auto with nothing: name=%q err=%v, want null", name, err)
	}
}

// TestOpenAlsaRoutesDeviceArg: the explicit "alsa" spec routes the configured
// device through the factory arg (D37), and errors when alsa won't open.
func TestOpenAlsaRoutesDeviceArg(t *testing.T) {
	setAvailable("alsa", true)
	b, name, err := OpenDevice("alsa", "hw:1,0", nil)
	if err != nil || name != "alsa" {
		t.Fatalf("OpenDevice(alsa, hw:1,0): name=%q err=%v", name, err)
	}
	defer b.Close()
	if ds, ok := b.(*discardSink); !ok || ds.arg != "hw:1,0" {
		t.Fatalf("alsa device not routed through the factory arg: %#v", b)
	}

	// Explicit alsa that won't open ⇒ error (not a silent degrade).
	setAvailable("alsa", false)
	if _, _, err := OpenDevice("alsa", "", nil); err == nil {
		t.Fatal("explicit alsa that cannot open should error")
	}
}

// TestOpenExecDegradesToNull: explicit "exec" with no usable player degrades to
// null with a warning rather than erroring.
func TestOpenExecDegradesToNull(t *testing.T) {
	setAvailable("exec", false)
	b, name, err := OpenDevice("exec", "", nil)
	if err != nil {
		t.Fatalf("explicit exec with no tool should degrade, not error: %v", err)
	}
	defer b.Close()
	if name != "null" {
		t.Fatalf("exec with no tool should degrade to null, got %q", name)
	}
	// exec available ⇒ exec selected.
	setAvailable("exec", true)
	if _, name, err := OpenDevice("exec", "", nil); err != nil || name != "exec" {
		t.Fatalf("explicit exec with tool: name=%q err=%v, want exec", name, err)
	}
}

func TestOpenResilientNullAndFileBypass(t *testing.T) {
	// null spec ⇒ bare null, no chain.
	b, name, err := OpenResilient("null", "", nil)
	if err != nil || name != "null" {
		t.Fatalf("OpenResilient(null): name=%q err=%v", name, err)
	}
	b.Close()
	if _, ok := b.(*resilientBackend); ok {
		t.Fatal("OpenResilient(null) must NOT return the resilient wrapper")
	}

	// file spec ⇒ bare file sink, no chain.
	path := filepath.Join(t.TempDir(), "r.pcm")
	b2, name2, err := OpenResilient("file:"+path, "", nil)
	if err != nil || name2 != "file" {
		t.Fatalf("OpenResilient(file): name=%q err=%v", name2, err)
	}
	b2.Close()
	if _, ok := b2.(*resilientBackend); ok {
		t.Fatal("OpenResilient(file) must NOT return the resilient wrapper")
	}
}

// TestOpenResilientAutoBuildsChain: with at least one failover candidate on the
// host, OpenResilient(auto) returns the resilient wrapper named "auto". We register
// a dedicated provider so the chain is guaranteed non-empty regardless of test order.
func TestOpenResilientAutoBuildsChain(t *testing.T) {
	putCtl("orc_a", &fakeCtl{})
	RegisterCandidates("named_explicit", func(string) []Candidate { return []Candidate{tkCand("orc_a")} })

	b, name, err := OpenResilient("auto", "", nil)
	if err != nil {
		t.Fatalf("OpenResilient(auto): %v", err)
	}
	defer b.Close()
	if name != "auto" {
		t.Fatalf("OpenResilient(auto) name=%q, want auto", name)
	}
	if _, ok := b.(*resilientBackend); !ok {
		t.Fatalf("OpenResilient(auto) returned %T, want *resilientBackend", b)
	}
}
