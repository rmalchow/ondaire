package sink

import (
	"testing"

	"ondaire/internal/stream"
)

func pcm(b byte) []byte {
	p := make([]byte, stream.FrameBytes)
	for i := range p {
		p[i] = b
	}
	return p
}

func TestJitterInsertPopInOrder(t *testing.T) {
	j := newJitterBuffer(16)
	j.setOrigin(0)
	for s := uint64(0); s < 5; s++ {
		if !j.insert(s, int64(s), pcm(byte(s))) {
			t.Fatalf("insert %d rejected", s)
		}
	}
	for s := uint64(0); s < 5; s++ {
		sl := j.pop(s)
		if sl == nil || sl.payload[0] != byte(s) {
			t.Fatalf("pop %d wrong: %v", s, sl)
		}
		j.advance()
	}
}

func TestJitterReorderRecovers(t *testing.T) {
	j := newJitterBuffer(16)
	j.setOrigin(0)
	j.insert(0, 0, pcm(0))
	j.insert(2, 2, pcm(2))
	j.insert(1, 1, pcm(1))
	for s := uint64(0); s < 3; s++ {
		sl := j.pop(s)
		if sl == nil || sl.payload[0] != byte(s) {
			t.Fatalf("reorder pop %d wrong", s)
		}
		j.advance()
	}
}

func TestJitterPopMissingReturnsNil(t *testing.T) {
	j := newJitterBuffer(16)
	j.setOrigin(0)
	j.insert(0, 0, pcm(0))
	if sl := j.pop(1); sl != nil {
		t.Fatal("missing seq should return nil")
	}
}

func TestJitterLateInsertRejected(t *testing.T) {
	j := newJitterBuffer(16)
	j.setOrigin(0)
	j.insert(0, 0, pcm(0))
	j.pop(0)
	j.advance() // nextSeq = 1
	if j.insert(0, 0, pcm(0)) {
		t.Fatal("late insert (seq < nextSeq) should be rejected")
	}
}

func TestJitterFullDropsFurthest(t *testing.T) {
	j := newJitterBuffer(2)
	j.setOrigin(0)
	if !j.insert(5, 5, pcm(5)) {
		t.Fatal("insert 5 rejected")
	}
	if !j.insert(6, 6, pcm(6)) {
		t.Fatal("insert 6 rejected")
	}
	// Full now. A nearer-future seq evicts the furthest (6).
	if !j.insert(3, 3, pcm(3)) {
		t.Fatal("nearer-future insert should evict furthest")
	}
	if j.pop(6) != nil {
		t.Fatal("furthest (6) should have been evicted")
	}
	if j.pop(3) == nil || j.pop(5) == nil {
		t.Fatal("3 and 5 should remain")
	}
	// A furthest-out seq when full is dropped.
	j2 := newJitterBuffer(2)
	j2.setOrigin(0)
	j2.insert(1, 1, pcm(1))
	j2.insert(2, 2, pcm(2))
	if j2.insert(9, 9, pcm(9)) {
		t.Fatal("furthest-out insert when full should be dropped")
	}
}

func TestJitterDuplicateOverwrites(t *testing.T) {
	j := newJitterBuffer(16)
	j.setOrigin(0)
	j.insert(0, 0, pcm(1))
	j.insert(0, 0, pcm(2)) // overwrite
	sl := j.pop(0)
	if sl == nil || sl.payload[0] != 2 {
		t.Fatal("duplicate seq should overwrite idempotently")
	}
}

func TestJitterResetClearsOrigin(t *testing.T) {
	j := newJitterBuffer(16)
	j.setOrigin(7)
	j.insert(7, 7, pcm(7))
	j.reset()
	if j.hasNext || j.len() != 0 {
		t.Fatal("reset should clear origin and empty buffer")
	}
}
