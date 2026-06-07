package sink

// slot holds one buffered frame's payload + pts. payload is owned (copied on
// Push) so the receiver may reuse its read buffer.
type slot struct {
	pts     int64
	payload []byte // exactly stream.FrameBytes
}

// jitterBuffer is a bounded seq-keyed reorder buffer. NOT goroutine-safe; the
// Playout mutex guards every call.
type jitterBuffer struct {
	slots   map[uint64]*slot
	cap     int
	nextSeq uint64 // seq the scheduler plays next
	hasNext bool   // false until the first frame fixes the seq origin
	maxSeq  uint64 // highest seq ever inserted (gap-vs-end watermark)
	hasMax  bool
}

func newJitterBuffer(capacity int) *jitterBuffer {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	return &jitterBuffer{slots: make(map[uint64]*slot), cap: capacity}
}

// setOrigin fixes nextSeq on the first frame of a session.
func (j *jitterBuffer) setOrigin(seq uint64) {
	j.nextSeq = seq
	j.hasNext = true
}

// insert stores a frame, returning false if it is rejected (already passed, or
// the buffer is full and seq is no nearer-future than its furthest slot).
// Duplicate seq overwrites idempotently.
func (j *jitterBuffer) insert(seq uint64, pts int64, payload []byte) bool {
	if j.hasNext && seq < j.nextSeq {
		return false // already passed → late
	}
	if !j.hasMax || seq > j.maxSeq {
		j.maxSeq = seq
		j.hasMax = true
	}
	if _, ok := j.slots[seq]; ok {
		// idempotent overwrite (FEC double-delivery)
		j.slots[seq] = &slot{pts: pts, payload: clone(payload)}
		return true
	}
	if len(j.slots) >= j.cap {
		// full: evict the furthest-future slot if this one is nearer.
		var furthest uint64
		first := true
		for s := range j.slots {
			if first || s > furthest {
				furthest = s
				first = false
			}
		}
		if seq >= furthest {
			return false // this frame is furthest-out; drop it
		}
		delete(j.slots, furthest)
	}
	j.slots[seq] = &slot{pts: pts, payload: clone(payload)}
	return true
}

// pop removes and returns the slot for seq, or nil if absent.
func (j *jitterBuffer) pop(seq uint64) *slot {
	s, ok := j.slots[seq]
	if !ok {
		return nil
	}
	delete(j.slots, seq)
	return s
}

// advance bumps nextSeq after a play or silence.
func (j *jitterBuffer) advance() {
	j.nextSeq++
}

// hasPending reports whether the scheduler still has frames to play up to the
// highest received seq. When false the buffer has drained to its end and the
// scheduler should go idle rather than synthesize silence past the last frame.
func (j *jitterBuffer) hasPending() bool {
	return j.hasNext && j.hasMax && j.nextSeq <= j.maxSeq
}

// reset empties the buffer and clears the seq origin (new generation).
func (j *jitterBuffer) reset() {
	j.slots = make(map[uint64]*slot)
	j.nextSeq = 0
	j.hasNext = false
	j.maxSeq = 0
	j.hasMax = false
}

func (j *jitterBuffer) len() int { return len(j.slots) }

func clone(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
