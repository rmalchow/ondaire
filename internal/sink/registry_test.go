package sink

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestOpenNull(t *testing.T) {
	b, name, err := Open("null", nil)
	if err != nil || name != "null" {
		t.Fatalf("Open(null): name=%q err=%v", name, err)
	}
	defer b.Close()
	if _, ok := b.(*nullBackend); !ok {
		t.Fatalf("expected *nullBackend, got %T", b)
	}
}

func TestOpenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.pcm")
	b, name, err := Open("file:"+path, nil)
	if err != nil || name != "file" {
		t.Fatalf("Open(file): name=%q err=%v", name, err)
	}
	defer b.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestOpenAutoDegradesToNull(t *testing.T) {
	// Empty PATH so no exec tool resolves; alsa may or may not be registered.
	t.Setenv("PATH", "")
	b, name, err := Open("auto", nil)
	if err != nil {
		t.Fatalf("auto should never error: %v", err)
	}
	defer b.Close()
	// With no PATH tools: null, unless alsa is loadable on this host.
	if name != "null" && name != "alsa" {
		t.Fatalf("auto with empty PATH resolved to %q", name)
	}
}

func TestOpenExecExplicitNoToolDegrades(t *testing.T) {
	t.Setenv("PATH", "")
	b, name, err := Open("exec", nil)
	if err != nil {
		t.Fatalf("explicit exec with no tool should degrade, not error: %v", err)
	}
	defer b.Close()
	if name != "null" {
		t.Fatalf("exec with no tool should degrade to null, got %q", name)
	}
}

func TestOpenUnknownErrors(t *testing.T) {
	if _, _, err := Open("bogus", nil); err == nil {
		t.Fatal("Open(bogus) should error")
	}
}

func TestOpenAlsaUnloadableErrors(t *testing.T) {
	if isRegistered("alsa") {
		t.Skip("libasound is loadable on this host; the unloadable path is not exercised here")
	}
	if _, _, err := Open("alsa", nil); err == nil {
		t.Fatal("Open(alsa) should error when alsa is not registered")
	}
	// auto must still succeed (skips alsa).
	t.Setenv("PATH", "")
	if _, name, err := Open("auto", nil); err != nil || name != "null" {
		t.Fatalf("auto should degrade to null without alsa: name=%q err=%v", name, err)
	}
}

func TestBackendNamesBaseSet(t *testing.T) {
	names := BackendNames()
	for _, want := range []string{"exec", "file", "null"} {
		if !slices.Contains(names, want) {
			t.Fatalf("BackendNames missing %q: %v", want, names)
		}
	}
	if !slices.IsSorted(names) {
		t.Fatalf("BackendNames not sorted: %v", names)
	}
}

func TestBackendNamesIncludesAlsaWhenLoaded(t *testing.T) {
	if !isRegistered("alsa") {
		t.Skip("libasound not loadable; alsa not registered")
	}
	if !slices.Contains(BackendNames(), "alsa") {
		t.Fatalf("alsa loadable but missing from BackendNames: %v", BackendNames())
	}
}

func TestHasPlayback(t *testing.T) {
	// Smoke: must not spawn, must not panic; value depends on host.
	_ = HasPlayback()
}
