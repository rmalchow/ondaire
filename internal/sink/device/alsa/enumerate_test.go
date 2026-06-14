package alsa

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseProcPCM exercises the pure /proc/asound/pcm parser: card-device →
// hw:C,D, the leading "default" entry, playback-only filtering, and the id field
// as the description. Ported from the old devices_test.go.
func TestParseProcPCM(t *testing.T) {
	const proc = `00-00: ALC892 Analog : ALC892 Analog : playback 1 : capture 1
00-01: ALC892 Digital : ALC892 Digital : playback 1
01-03: HDMI 0 : HDMI 0 : playback 1
02-00: USB Audio : USB Audio : capture 1
`
	got := parseProcPCM(proc)
	want := []struct{ id, desc string }{
		{"default", "system default"},
		{"hw:0,0", "ALC892 Analog"},
		{"hw:0,1", "ALC892 Digital"},
		{"hw:1,3", "HDMI 0"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d devices, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].ID != w.id || got[i].Desc != w.desc {
			t.Errorf("device[%d] = {%q,%q}, want {%q,%q}", i, got[i].ID, got[i].Desc, w.id, w.desc)
		}
	}
}

func TestParseProcPCMEmptyOrCaptureOnly(t *testing.T) {
	// Empty content ⇒ nil (no lone "default").
	if got := parseProcPCM(""); got != nil {
		t.Errorf("empty proc: got %+v, want nil", got)
	}
	// Capture-only host yields no playback device ⇒ nil.
	if got := parseProcPCM("02-00: USB Mic : USB Mic : capture 1\n"); got != nil {
		t.Errorf("capture-only: got %+v, want nil", got)
	}
}

func TestParseProcPCMMalformedLinesSkipped(t *testing.T) {
	const proc = `garbage without colon
: : :
00-00: Card : Card : playback 1
`
	got := parseProcPCM(proc)
	if len(got) != 2 || got[0].ID != "default" || got[1].ID != "hw:0,0" {
		t.Fatalf("got %+v, want [default hw:0,0]", got)
	}
}

// TestParseProcPCMCardDeviceZeroPadding verifies the zero-stripping of the card
// and device tokens (and the "all zeros" → "0" fallback).
func TestParseProcPCMCardDeviceZeroPadding(t *testing.T) {
	const proc = `10-02: Card Ten : Card Ten : playback 1
00-00: Card Zero : Card Zero : playback 1
`
	got := parseProcPCM(proc)
	if len(got) != 3 {
		t.Fatalf("got %d devices, want 3: %+v", len(got), got)
	}
	if got[1].ID != "hw:10,2" {
		t.Errorf("device[1].ID=%q, want hw:10,2", got[1].ID)
	}
	if got[2].ID != "hw:0,0" {
		t.Errorf("device[2].ID=%q, want hw:0,0 (all-zero card/dev)", got[2].ID)
	}
}

// TestListOutputDevicesGatedByBound proves the enumerator returns nothing when
// the libasound probe never bound, regardless of /proc contents — and reads the
// overridable procPCMPath when it did. Mutates package globals; restored on exit.
func TestListOutputDevicesGatedByBound(t *testing.T) {
	// Build a fixture proc file and point the parser at it.
	dir := t.TempDir()
	procFile := filepath.Join(dir, "pcm")
	const proc = "00-00: Fixture Card : Fixture Card : playback 1\n"
	if err := os.WriteFile(procFile, []byte(proc), 0o644); err != nil {
		t.Fatal(err)
	}

	savedBound, savedPath := bound, procPCMPath
	t.Cleanup(func() { bound, procPCMPath = savedBound, savedPath })
	procPCMPath = procFile

	// libasound not bound ⇒ no enumeration even though the proc file lists a device.
	bound = nil
	if got := ListOutputDevices(); got != nil {
		t.Fatalf("unbound libasound should enumerate nothing, got %+v", got)
	}

	// Bound ⇒ the fixture is parsed.
	bound = &funcs{}
	got := ListOutputDevices()
	if len(got) != 2 || got[0].ID != "default" || got[1].ID != "hw:0,0" {
		t.Fatalf("bound enumeration got %+v, want [default hw:0,0]", got)
	}
}

// TestCandidatesProviderOrder verifies the alsa candidate provider: preferred
// device first, then "default", then enumerated hw devices, deduped stable.
func TestCandidatesProviderOrder(t *testing.T) {
	dir := t.TempDir()
	procFile := filepath.Join(dir, "pcm")
	const proc = "00-00: A : A : playback 1\n01-00: B : B : playback 1\n"
	if err := os.WriteFile(procFile, []byte(proc), 0o644); err != nil {
		t.Fatal(err)
	}
	savedBound, savedPath := bound, procPCMPath
	t.Cleanup(func() { bound, procPCMPath = savedBound, savedPath })
	procPCMPath = procFile
	bound = &funcs{}

	got := candidates("hw:1,0") // operator selected hw:1,0
	// Expect: hw:1,0 (preferred), default, hw:0,0 — hw:1,0 deduped from the
	// enumeration tail.
	wantArgs := []string{"hw:1,0", "default", "hw:0,0"}
	if len(got) != len(wantArgs) {
		t.Fatalf("got %d candidates, want %d: %+v", len(got), len(wantArgs), got)
	}
	for i, w := range wantArgs {
		if got[i].Arg != w {
			t.Errorf("candidate[%d].Arg=%q, want %q", i, got[i].Arg, w)
		}
		if got[i].Kind != "alsa" {
			t.Errorf("candidate[%d].Kind=%q, want alsa", i, got[i].Kind)
		}
	}
}
