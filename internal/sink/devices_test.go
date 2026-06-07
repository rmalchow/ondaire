package sink

import "testing"

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
	if got := parseProcPCM(""); got != nil {
		t.Errorf("empty proc: got %+v, want nil", got)
	}
	// A capture-only host yields no playback device → empty list (not lone default).
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
