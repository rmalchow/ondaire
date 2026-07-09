package source

import (
	"time"

	"ondaire/internal/stream"
)

// primePace is the inter-frame delay for a prime burst: ~5 ms/frame, i.e. ~4×
// realtime so the burst outruns the live stream (a 20 ms frame every 5 ms)
// without flooding (D24). Applied on BOTH transports: UDP paces its sendto, and
// TCP paces its blocking write so a weak link surfaces as the write falling
// behind the pace (→ catch-up abandon, below) instead of silently bloating the
// kernel send buffer + AP queue.
const primePace = 5 * time.Millisecond

// maxPrimeRounds is a hard backstop on prime catch-up rounds. Each round re-sends
// the frames released while the previous round was in flight; on a healthy link
// (paced ~4× realtime) this converges in a handful of rounds. The primary bound is
// the realtime check in prime() — this cap just guards against pathological
// oscillation. On abandon the subscriber resumes at the live edge and its sink
// re-anchors over the brief gap.
const maxPrimeRounds = 6

// prime bursts the selected ring frames to a (re)joining subscriber, then
// keeps catching up via the ring until it reaches the live edge, and only
// then re-admits the subscriber to live fan-out (clears sub.priming). This
// guarantees the subscriber sees seqs from the oldest primed frame onward
// with no interleaved newer frame — the client reorder window and the sink
// anchor on the burst, never on a racing live frame. The burst runs at >
// realtime (UDP ~4×, TCP back-to-back), so catch-up always terminates.
// Runs in its own goroutine (wg-tracked). No FEC during a prime — the live
// FEC cadence is independent. The Primes stat is counted at initiation
// (onSubscribe), not here.
func (s *Server) prime(sub *subscriber, frames []ringSlot, gen uint32) {
	defer s.wg.Done()
	donePriming := func() {
		s.mu.Lock()
		sub.priming = false
		s.mu.Unlock()
	}
	round := 0
	sentFrames := 0        // total frames pushed across rounds
	start := time.Now()    // wall clock for the realtime-keeping check
	for {
		round++
		s.log.Debug("prime catch-up round", "addr", sub.addr.String(), "round", round, "frames", len(frames), "gen", gen)
		var ok bool
		if sub.tr == stream.TransportTCP {
			ok = s.primeTCP(sub, frames, gen)
		} else {
			ok = s.primeUDP(sub, frames, gen)
		}
		if !ok { // server closing or conn dead (incl. write-deadline timeout) — give up, re-admit
			donePriming()
			return
		}
		sentFrames += len(frames)
		last := frames[len(frames)-1].seq
		s.mu.Lock()
		if !s.active || s.gen != gen {
			// Session ended or superseded mid-prime; the new generation's
			// frames reach the sub via live fan-out (and RECONFIG handling).
			sub.priming = false
			s.mu.Unlock()
			return
		}
		frames = s.ring.framesAfter(last)
		if len(frames) == 0 {
			sub.priming = false // caught up: live from the next ReleaseFrame
			s.mu.Unlock()
			return
		}
		// Bounded catch-up: stop rather than flood a weak link into bufferbloat.
		// If the cumulative send has fallen behind realtime, the subscriber drains
		// slower than the stream produces — catch-up can never converge and would
		// pile unbounded data onto its RX path (multi-second RTT → watchdog reset).
		// The paced send is ~4× realtime, so a healthy link stays well ahead; only a
		// genuinely slow link trips this. maxPrimeRounds is a hard backstop. Either
		// way we re-admit to live and the sink re-anchors over the brief gap.
		behindRealtime := time.Since(start) > time.Duration(int64(sentFrames)*stream.FrameNanos)
		if behindRealtime || round >= maxPrimeRounds {
			sub.priming = false
			s.mu.Unlock()
			s.log.Warn("prime abandoned to live edge; subscriber slower than realtime",
				"addr", sub.addr.String(), "round", round, "sentFrames", sentFrames,
				"elapsed", time.Since(start), "backlog", len(frames), "gen", gen)
			return
		}
		s.mu.Unlock()
	}
}

// primeUDP paces TypeAudio datagrams to a UDP subscriber, one per ~5 ms, each
// carrying its original Seq/PTS/Gen. Returns false if the server is closing.
func (s *Server) primeUDP(sub *subscriber, frames []ringSlot, gen uint32) bool {
	t := time.NewTicker(primePace)
	defer t.Stop()
	buf := make([]byte, 0, stream.HeaderSize+stream.FrameBytes)
	for _, f := range frames {
		select {
		case <-s.done:
			return false
		case <-t.C:
		}
		h := stream.Header{
			Magic:      stream.Magic,
			Type:       stream.TypeAudio,
			Gen:        gen,
			Seq:        f.seq,
			PTS:        f.pts,
			PayloadLen: uint16(len(f.payload)),
		}
		pkt := h.AppendFrame(buf[:0], f.payload)
		s.writeUDP(pkt, sub.addr)
	}
	return true
}

// primeTCP writes the selected frames (length-prefixed) on the subscriber's conn
// under its wmu, paced at ~primePace/frame (~4× realtime) rather than dumping
// them back-to-back. Pacing keeps the burst from over-filling the kernel send
// buffer + AP queue at once, and makes a weak link show up as the write blocking
// past the pace (and, past tcpWriteTimeout, as a write error that aborts prime)
// instead of silent bloat. The sub is excluded from live fan-out while priming,
// so holding wmu across the paced burst starves nothing. Returns false if the
// server is closing or the conn died.
func (s *Server) primeTCP(sub *subscriber, frames []ringSlot, gen uint32) bool {
	sub.wmu.Lock()
	defer sub.wmu.Unlock()
	if sub.conn == nil || sub.dead.Load() {
		return false
	}
	t := time.NewTicker(primePace)
	defer t.Stop()
	buf := make([]byte, 0, stream.HeaderSize+stream.FrameBytes)
	for _, f := range frames {
		select {
		case <-s.done:
			return false
		case <-t.C:
		}
		h := stream.Header{
			Magic:      stream.Magic,
			Type:       stream.TypeAudio,
			Gen:        gen,
			Seq:        f.seq,
			PTS:        f.pts,
			PayloadLen: uint16(len(f.payload)),
		}
		pkt := h.AppendFrame(buf[:0], f.payload)
		_ = sub.conn.SetWriteDeadline(time.Now().Add(tcpWriteTimeout))
		if err := writeTCPFrame(sub.conn, pkt); err != nil {
			sub.dead.Store(true)
			_ = sub.conn.Close()
			return false
		}
	}
	return true
}
