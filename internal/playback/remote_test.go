package playback

import (
	"net/netip"
	"sync"
	"testing"

	"ondaire/internal/stream"
)

type capturingWriter struct {
	mu     sync.Mutex
	writes []struct {
		pkt []byte
		dst netip.AddrPort
	}
}

func (w *capturingWriter) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error) {
	w.mu.Lock()
	w.writes = append(w.writes, struct {
		pkt []byte
		dst netip.AddrPort
	}{append([]byte(nil), b...), addr})
	w.mu.Unlock()
	return len(b), nil
}

func newRemote(t *testing.T) (Player, *capturingWriter, netip.AddrPort) {
	t.Helper()
	w := &capturingWriter{}
	dst := netip.MustParseAddrPort("10.0.0.7:9300")
	return NewRemote(w, dst, nil), w, dst
}

func lastFrame(t *testing.T, w *capturingWriter, wantDst netip.AddrPort) (stream.Header, []byte) {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.writes) == 0 {
		t.Fatal("no control packet sent")
	}
	last := w.writes[len(w.writes)-1]
	if last.dst != wantDst {
		t.Fatalf("dst = %v, want %v", last.dst, wantDst)
	}
	h, payload, err := stream.DecodeFrame(last.pkt)
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	return h, payload
}

func TestRemoteAttachEncodes(t *testing.T) {
	r, w, dst := newRemote(t)
	a := Attach{
		Source:    netip.MustParseAddrPort("10.0.0.1:9200"),
		Clock:     netip.MustParseAddrPort("10.0.0.1:9090"),
		Codec:     stream.CodecOpus,
		Transport: stream.TransportTCP,
		BufferMs:  150,
	}
	r.Attach(a)
	h, payload := lastFrame(t, w, dst)
	if h.Type != stream.TypeAttach {
		t.Fatalf("type = 0x%02x, want ATTACH", h.Type)
	}
	got, err := stream.DecodeAttach(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != a.Source || got.Clock != a.Clock || got.Codec != stream.CodecOpus ||
		got.Transport != stream.TransportTCP || got.BufferMs != 150 {
		t.Fatalf("attach payload: %+v", got)
	}
}

func TestRemoteDetachEncodes(t *testing.T) {
	r, w, dst := newRemote(t)
	r.Detach()
	h, payload := lastFrame(t, w, dst)
	if h.Type != stream.TypeDetach || len(payload) != 0 {
		t.Fatalf("detach: type=0x%02x len=%d", h.Type, len(payload))
	}
}

func TestRemoteSetVolEncodes(t *testing.T) {
	r, w, dst := newRemote(t)
	r.SetVolume(73, true)
	h, payload := lastFrame(t, w, dst)
	if h.Type != stream.TypeSetVol {
		t.Fatalf("type = 0x%02x, want SETVOL", h.Type)
	}
	v, _ := stream.DecodeSetVol(payload)
	if v.VolumePct != 73 || !v.Mute {
		t.Fatalf("setvol: %+v", v)
	}
}

func TestRemoteSetDelayEncodes(t *testing.T) {
	r, w, dst := newRemote(t)
	r.SetDelay(-40)
	h, payload := lastFrame(t, w, dst)
	if h.Type != stream.TypeSetDelay {
		t.Fatalf("type = 0x%02x, want SETDELAY", h.Type)
	}
	d, _ := stream.DecodeSetDelay(payload)
	if d.DelayMs != -40 {
		t.Fatalf("setdelay = %d, want -40", d.DelayMs)
	}
}

func TestRemoteSetCapEncodes(t *testing.T) {
	r, w, dst := newRemote(t)
	r.SetCap(5, true)
	h, payload := lastFrame(t, w, dst)
	if h.Type != stream.TypeSetCap {
		t.Fatalf("type = 0x%02x, want SETCAP", h.Type)
	}
	c, _ := stream.DecodeSetCap(payload)
	if c.CapID != 5 || !c.On {
		t.Fatalf("setcap: %+v", c)
	}
}

func TestRemoteSyncIsNoOp(t *testing.T) {
	r, w, _ := newRemote(t)
	r.Sync(netip.MustParseAddrPort("10.0.0.1:9090"), 3)
	if len(w.writes) != 0 {
		t.Fatal("Sync must not send anything on the remote player")
	}
	if got := r.Status(); got != (stream.StatusPayload{}) {
		t.Fatal("remote Status must be the zero value (it arrives async at the source)")
	}
}

var _ Player = (*remotePlayer)(nil)
