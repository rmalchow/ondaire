package playback

import (
	"log/slog"
	"net"
	"net/netip"

	"ensemble/internal/stream"
)

// controlWriter is the slice of *net.UDPConn the remotePlayer needs to send control
// packets. *net.UDPConn satisfies it; tests inject a fake.
type controlWriter interface {
	WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error)
}

// The master's control socket K injects must satisfy controlWriter.
var _ controlWriter = (*net.UDPConn)(nil)

// remotePlayer is the Player a MASTER drives over the wire for a non-gossiping
// playback node (D58/D61): each verb is encoded as a control packet (control.go)
// and sent to the node's CONTROL_PORT. It is the wire counterpart of localPlayer.
//
// The control plane is soft-state (D58): the master's control driver re-asserts
// ATTACH/SETVOL/SETDELAY on a heartbeat, so a lost datagram self-heals. remotePlayer
// is just the encoder/sender; it holds no state and is safe for concurrent sends.
type remotePlayer struct {
	w   controlWriter
	dst netip.AddrPort // the playback node's CONTROL_PORT endpoint
	log *slog.Logger
}

// NewRemote builds a wire-driving Player targeting a playback node's control
// endpoint. w is the master's shared control-sending socket.
func NewRemote(w controlWriter, dst netip.AddrPort, log *slog.Logger) Player {
	if log == nil {
		log = slog.Default()
	}
	return &remotePlayer{w: w, dst: dst, log: log.With("comp", "pb-remote")}
}

func (r *remotePlayer) send(typ byte, payload []byte) {
	h := stream.Header{Magic: stream.Magic, Type: typ}
	pkt := h.AppendFrame(make([]byte, 0, stream.HeaderSize+len(payload)), payload)
	if _, err := r.w.WriteToUDPAddrPort(pkt, r.dst); err != nil {
		r.log.Debug("control send failed", "type", typ, "to", r.dst, "err", err)
	}
}

func (r *remotePlayer) Attach(a Attach) {
	p := stream.AttachPayload{
		Source:    a.Source,
		Clock:     a.Clock,
		Codec:     a.Codec,
		Transport: a.Transport,
		BufferMs:  uint16(a.BufferMs),
	}
	r.send(stream.TypeAttach, p.AppendTo(nil))
}

func (r *remotePlayer) Detach() { r.send(stream.TypeDetach, nil) }

// Sync is a no-op for a remote node: it follows the clock endpoint given in ATTACH
// itself (the master does not drive the node's clock follower).
func (r *remotePlayer) Sync(netip.AddrPort, uint32) {}

func (r *remotePlayer) SetVolume(pct uint8, mute bool) {
	r.send(stream.TypeSetVol, stream.SetVolPayload{VolumePct: pct, Mute: mute}.AppendTo(nil))
}

func (r *remotePlayer) SetDelay(ms int) {
	r.send(stream.TypeSetDelay, stream.SetDelayPayload{DelayMs: int16(ms)}.AppendTo(nil))
}

func (r *remotePlayer) SetEqualize(ms int) {
	if ms < 0 {
		ms = 0
	}
	r.send(stream.TypeSetEq, stream.SetEqualizePayload{DelayMs: uint16(ms)}.AppendTo(nil))
}

func (r *remotePlayer) SetCap(capID uint8, on bool) {
	r.send(stream.TypeSetCap, stream.SetCapPayload{CapID: capID, On: on}.AppendTo(nil))
}

// Status is not available synchronously on the master side: a playback node's
// STATUS arrives asynchronously at the source server (D55), not via this object.
// Returns the zero value; the driver reads source.Server.Statuses() instead.
func (r *remotePlayer) Status() stream.StatusPayload { return stream.StatusPayload{} }
