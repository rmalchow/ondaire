package group

import (
	"sync"
	"time"

	"ensemble/internal/contracts"
	"ensemble/internal/id"
)

// testRig bundles an Engine with all its fakes for easy assertion.
type testRig struct {
	e   *Engine
	cl  *fakeCluster
	med *fakeMedia
	srv *fakeSourceServer
	op  *fakeOpusFactory

	nowMu sync.Mutex
	now   time.Time

	persistMu sync.Mutex
	persisted []id.ID // every PersistFollowing(target) the engine drove (D45)
}

// newRig builds a rig whose self is `self`, with a pull source of n frames.
func newRig(self id.ID, srcFrames int, live bool) *testRig {
	r := &testRig{
		cl:  newFakeCluster(self),
		med: &fakeMedia{src: &fakeSource{remaining: srcFrames, live: live}},
		srv: &fakeSourceServer{},
		op:  &fakeOpusFactory{},
		now: time.Unix(1_700_000_000, 0),
	}
	r.e = New(Params{
		Cluster:   r.cl,
		Media:     r.med,
		Opus:      r.op,
		Source:    r.srv,
		Caps:      contracts.Capabilities{Codecs: []string{"pcm"}},
		now:       r.nowFn(),
		nowMaster: r.nowMasterFn(),
		PersistFollowing: func(t id.ID) {
			r.persistMu.Lock()
			r.persisted = append(r.persisted, t)
			r.persistMu.Unlock()
		},
	})
	return r
}

// nowMasterFn is the deterministic master-clock seam: the engine OWNS the clock,
// so master-time now is just the rig's fake wall clock in nanos. This makes
// startMaster predictable (startMaster == nowMaster() at install time).
func (r *testRig) nowMasterFn() func() int64 {
	return func() int64 {
		r.nowMu.Lock()
		defer r.nowMu.Unlock()
		return r.now.UnixNano()
	}
}

// lastPersisted returns the most recent PersistFollowing target the engine drove.
func (r *testRig) lastPersisted() (id.ID, bool) {
	r.persistMu.Lock()
	defer r.persistMu.Unlock()
	if len(r.persisted) == 0 {
		return id.Zero, false
	}
	return r.persisted[len(r.persisted)-1], true
}

func (r *testRig) nowFn() func() time.Time {
	return func() time.Time {
		r.nowMu.Lock()
		defer r.nowMu.Unlock()
		return r.now
	}
}

func (r *testRig) advance(d time.Duration) {
	r.nowMu.Lock()
	r.now = r.now.Add(d)
	r.nowMu.Unlock()
}

// --- snapshot builders -------------------------------------------------------

func node(nid id.ID, following id.ID, alive bool) contracts.NodeView {
	return contracts.NodeView{
		ID:           nid,
		Name:         nid.String()[:8],
		Following:    following,
		Alive:        alive,
		SourcePort:   9200,
		StreamPort:   9090,
		Capabilities: contracts.Capabilities{Codecs: []string{"pcm"}},
	}
}

// soloSnap makes self the master of its own group, playing its own stream
// (Following == self → its player is in its own group). New model: a node plays
// its own group by following itself.
func soloSnap(self id.ID) contracts.Snapshot {
	n := node(self, self, true) // Following == self: plays own group
	g := contracts.GroupView{
		ID:       self, // group id == master id (D44)
		Master:   self,
		Members:  []id.ID{self}, // the master's own player
		Settings: defaultSettings(),
		Playback: contracts.Playback{State: "idle"},
	}
	return contracts.Snapshot{Nodes: []contracts.NodeView{n}, Groups: []contracts.GroupView{g}}
}

// masterSnap makes `master` source a group whose players are the master itself
// (self-follow) plus `members` (each following the master). New model: Members are
// the PLAYERS following the master; the master is a member because it self-follows.
func masterSnap(master id.ID, settings contracts.GroupSettings, members ...id.ID) contracts.Snapshot {
	all := append([]id.ID{master}, members...)
	var nodes []contracts.NodeView
	nodes = append(nodes, node(master, master, true)) // master plays its own group
	for _, m := range members {
		nodes = append(nodes, node(m, master, true))
	}
	// Every alive node masters its OWN group (1:1). group(master) holds all players;
	// each member also masters its own (empty) group.
	groups := []contracts.GroupView{{
		ID:       master,
		Master:   master,
		Members:  all,
		Settings: settings,
		Playback: contracts.Playback{State: "idle"},
	}}
	for _, m := range members {
		groups = append(groups, contracts.GroupView{
			ID:       m,
			Master:   m,
			Members:  nil,
			Settings: defaultSettings(),
			Playback: contracts.Playback{State: "idle"},
		})
	}
	return contracts.Snapshot{Nodes: nodes, Groups: groups}
}

// idN returns a deterministic non-zero ID for tests.
func idN(b byte) id.ID {
	var i id.ID
	i[15] = b
	return i
}

// withPlaying marks every group in the snapshot as having an active session,
// so repoint tests exercise the session-gated subscribe/arm path.
func withPlaying(s contracts.Snapshot) contracts.Snapshot {
	for i := range s.Groups {
		s.Groups[i].Playback.State = "playing"
	}
	return s
}
