package cluster

import (
	"net/netip"
	"strings"
	"testing"

	"ondaire/internal/contracts"
	"ondaire/internal/id"
)

// findGroup returns the derived group mastered by `master`, or nil.
func findGroup(groups []contracts.GroupView, master id.ID) *contracts.GroupView {
	for i := range groups {
		if groups[i].Master == master {
			return &groups[i]
		}
	}
	return nil
}

func TestSnapshotResolvesLiveness(t *testing.T) {
	self := id.New()
	peer := id.New()
	c := newTestCluster(t, self, nil)
	c.mu.Lock()
	c.doc.Nodes[peer] = nodeRec(peer, 1, "peer")
	c.mu.Unlock()
	c.live.join(peer, 1000)

	snap := c.Snapshot()
	var selfAlive, peerAlive bool
	for _, n := range snap.Nodes {
		if n.ID == self {
			selfAlive = n.Alive
		}
		if n.ID == peer {
			peerAlive = n.Alive
		}
	}
	if !selfAlive {
		t.Fatal("self should be alive")
	}
	if !peerAlive {
		t.Fatal("joined peer should be alive")
	}
}

func TestSnapshotDeepCopy(t *testing.T) {
	self := id.New()
	c := newTestCluster(t, self, nil)
	snap := c.Snapshot()
	snap.Nodes[0].Name = "mutated"
	if c.Snapshot().Nodes[0].Name == "mutated" {
		t.Fatal("snapshot shares state with live doc")
	}
}

// New grouping model: every alive node is ALWAYS the master of its own group
// (1:1); `following` only places the node's PLAYER. following==Zero ⇒ idle (no
// group); following==self ⇒ play own group; following==other ⇒ crosswise. Members
// of a group are its PLAYERS (the master is a member only if it follows itself).

func TestDeriveGroupsIdleNodeIsEmptyOwnGroup(t *testing.T) {
	self := id.New()
	nodes := map[id.ID]*NodeRecord{self: {ID: self, Following: id.Zero}}
	groups := DeriveGroups(nodes, nil, nil, nil, map[id.ID]bool{}, self)
	if len(groups) != 1 {
		t.Fatalf("want 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.ID != self || g.Master != self {
		t.Fatalf("group id/master should equal node id: %+v", g)
	}
	if len(g.Members) != 0 {
		t.Fatalf("an idle node is its own EMPTY group; got %d members", len(g.Members))
	}
}

func TestDeriveGroupsPlayersJoinMaster(t *testing.T) {
	a := id.New()
	b := id.New()
	nodes := map[id.ID]*NodeRecord{
		a: {ID: a, Following: a}, // a plays its own group
		b: {ID: b, Following: a}, // b's player joins a's group
	}
	alive := map[id.ID]bool{a: true, b: true}
	groups := DeriveGroups(nodes, nil, nil, nil, alive, a)
	// Every alive node masters its own group → 2 groups (a has [a,b]; b empty).
	if len(groups) != 2 {
		t.Fatalf("want 2 groups (one per master), got %d", len(groups))
	}
	ga := findGroup(groups, a)
	if ga == nil || len(ga.Members) != 2 {
		t.Fatalf("group a should have players [a,b]: %+v", ga)
	}
	if gb := findGroup(groups, b); gb == nil || len(gb.Members) != 0 {
		t.Fatalf("group b should be empty: %+v", gb)
	}
}

func TestDeriveGroupsDeadTargetIdle(t *testing.T) {
	a := id.New() // dead
	b := id.New() // b's player targets dead a → idle
	nodes := map[id.ID]*NodeRecord{
		a: {ID: a, Following: id.Zero},
		b: {ID: b, Following: a},
	}
	alive := map[id.ID]bool{b: true} // a is dead
	groups := DeriveGroups(nodes, nil, nil, nil, alive, b)
	if len(groups) != 1 { // only b masters a group (a is dead)
		t.Fatalf("want 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.Master != b || len(g.Members) != 0 {
		t.Fatalf("b's player targets a dead master → idle empty group: %+v", g)
	}
}

func TestDeriveGroupsCrosswise(t *testing.T) {
	a := id.New()
	b := id.New()
	nodes := map[id.ID]*NodeRecord{
		a: {ID: a, Following: b}, // a's speakers play b's stream
		b: {ID: b, Following: a}, // b's speakers play a's stream
	}
	alive := map[id.ID]bool{a: true, b: true}
	groups := DeriveGroups(nodes, nil, nil, nil, alive, a)
	if len(groups) != 2 {
		t.Fatalf("want 2 groups, got %d", len(groups))
	}
	ga, gb := findGroup(groups, a), findGroup(groups, b)
	if ga == nil || len(ga.Members) != 1 || ga.Members[0] != b {
		t.Fatalf("group a should have player b: %+v", ga)
	}
	if gb == nil || len(gb.Members) != 1 || gb.Members[0] != a {
		t.Fatalf("group b should have player a: %+v", gb)
	}
}

func TestDeriveGroupsStableOrder(t *testing.T) {
	a := id.New()
	b := id.New()
	nodes := map[id.ID]*NodeRecord{
		a: {ID: a, Following: id.Zero},
		b: {ID: b, Following: id.Zero},
	}
	alive := map[id.ID]bool{a: true, b: true}
	g1 := DeriveGroups(nodes, nil, nil, nil, alive, a)
	g2 := DeriveGroups(nodes, nil, nil, nil, alive, a)
	for i := range g1 {
		if g1[i].ID != g2[i].ID {
			t.Fatal("group order not deterministic")
		}
	}
	if len(g1) == 2 && idLess(g1[1].ID, g1[0].ID) {
		t.Fatal("groups not sorted by id")
	}
}

// Records (name override, playback, settings) are keyed by the MASTER/group id.
func TestDeriveGroupsRecordsKeyedByMaster(t *testing.T) {
	a := id.New()
	b := id.New()
	nodes := map[id.ID]*NodeRecord{
		a: {ID: a, Name: "alpha", Following: a},
		b: {ID: b, Name: "bravo", Following: a},
	}
	alive := map[id.ID]bool{a: true, b: true}
	names := map[id.ID]*GroupNameRecord{a: {Name: "Den"}}          // override keyed by master id
	playback := map[id.ID]*PlaybackRecord{a: {State: "playing"}}   // keyed by master id
	settings := map[id.ID]*GroupSettingsRecord{a: {Codec: "opus"}} // keyed by master id
	g := findGroup(DeriveGroups(nodes, names, playback, settings, alive, a), a)
	if g == nil || g.ID != a {
		t.Fatalf("no group a / wrong id: %+v", g)
	}
	if g.Name != "Den" || g.NameIsDerived {
		t.Fatalf("master-keyed override name not resolved: %q derived=%v", g.Name, g.NameIsDerived)
	}
	if g.Playback.State != "playing" {
		t.Fatalf("playback (master-keyed) not resolved: %+v", g.Playback)
	}
	if g.Settings.Codec != "opus" {
		t.Fatalf("settings (master-keyed) not resolved: %+v", g.Settings)
	}
}

// Derived label: sorted PLAYER names joined with " + "; an empty group falls back
// to the master's own room name.
func TestDeriveGroupsDerivedLabel(t *testing.T) {
	a := id.New()
	b := id.New()
	nodes := map[id.ID]*NodeRecord{
		a: {ID: a, Name: "kitchen", Following: a},
		b: {ID: b, Name: "bedroom", Following: a},
	}
	alive := map[id.ID]bool{a: true, b: true}
	g := findGroup(DeriveGroups(nodes, nil, nil, nil, alive, a), a)
	if g.Name != "bedroom + kitchen" { // player names, sorted
		t.Fatalf("derived label = %q", g.Name)
	}
	if !g.NameIsDerived {
		t.Fatal("derived label must be flagged NameIsDerived")
	}

	// A node playing its own group alone → label = its name.
	soloNodes := map[id.ID]*NodeRecord{a: {ID: a, Name: "kitchen", Following: a}}
	solo := findGroup(DeriveGroups(soloNodes, nil, nil, nil, map[id.ID]bool{a: true}, a), a)
	if solo.Name != "kitchen" || !solo.NameIsDerived {
		t.Fatalf("solo label = %q derived=%v", solo.Name, solo.NameIsDerived)
	}
}

// An empty group (no players) is labelled by the master's own room name.
func TestDeriveGroupsEmptyGroupLabelledByMaster(t *testing.T) {
	a := id.New()
	nodes := map[id.ID]*NodeRecord{a: {ID: a, Name: "kitchen", Following: id.Zero}}
	g := findGroup(DeriveGroups(nodes, nil, nil, nil, map[id.ID]bool{a: true}, a), a)
	if len(g.Members) != 0 {
		t.Fatalf("idle node = empty group, got %d members", len(g.Members))
	}
	if g.Name != "kitchen" || !g.NameIsDerived {
		t.Fatalf("empty group should be labelled by the master's name: %q", g.Name)
	}
}

// " +N more" truncation + short-id fallback for an unnamed player.
func TestDeriveGroupsDerivedLabelCapAndFallback(t *testing.T) {
	master := id.New()
	nodes := map[id.ID]*NodeRecord{master: {ID: master, Name: "m", Following: master}}
	for i := 0; i < 5; i++ {
		f := id.New()
		nodes[f] = &NodeRecord{ID: f, Name: "", Following: master} // empty name → short-id fallback
	}
	alive := map[id.ID]bool{}
	for nid := range nodes {
		alive[nid] = true
	}
	g := findGroup(DeriveGroups(nodes, nil, nil, nil, alive, master), master)
	if len(g.Members) != 6 { // master self-follows + 5 players
		t.Fatalf("want 6 members, got %d", len(g.Members))
	}
	if !strings.Contains(g.Name, "+3 more") { // 6 names → first 3 + " +3 more"
		t.Fatalf("expected truncation in label, got %q", g.Name)
	}
	if !g.NameIsDerived {
		t.Fatal("derived label must be flagged NameIsDerived")
	}
}

func TestDialCandidatesIntersection(t *testing.T) {
	self := id.New()
	peer := id.New()
	other := id.New()
	c := newTestCluster(t, self, nil)
	c.mu.Lock()
	// peer self-reports two IPs
	c.doc.Nodes[peer] = &NodeRecord{ID: peer, Addrs: []string{"10.0.0.5/24", "10.0.0.6/24"}}
	// self observed 10.0.0.5 recently; other observed 10.0.0.6 earlier
	c.doc.Nodes[self].Observed = map[id.ID]obsEntry{peer: {IP: "10.0.0.5", LastSeenUnix: 200}}
	c.doc.Nodes[other] = &NodeRecord{ID: other, Observed: map[id.ID]obsEntry{peer: {IP: "10.0.0.6", LastSeenUnix: 100}}}
	c.mu.Unlock()

	got := c.DialCandidates(peer)
	if len(got) != 2 {
		t.Fatalf("want 2 candidates, got %d: %v", len(got), got)
	}
	if got[0] != netip.MustParseAddr("10.0.0.5") {
		t.Fatalf("most-recent first failed: %v", got)
	}
}

func TestDialCandidatesFallbackEmptyObservations(t *testing.T) {
	self := id.New()
	peer := id.New()
	c := newTestCluster(t, self, nil)
	c.mu.Lock()
	c.doc.Nodes[peer] = &NodeRecord{ID: peer, Addrs: []string{"192.168.0.10/24"}}
	c.mu.Unlock()
	got := c.DialCandidates(peer)
	if len(got) != 1 || got[0] != netip.MustParseAddr("192.168.0.10") {
		t.Fatalf("cold-peer fallback failed: %v", got)
	}
}

func TestDialCandidatesSkipsUnparseableCIDR(t *testing.T) {
	self := id.New()
	peer := id.New()
	c := newTestCluster(t, self, nil)
	c.mu.Lock()
	c.doc.Nodes[peer] = &NodeRecord{ID: peer, Addrs: []string{"garbage", "10.1.1.1/24"}}
	c.mu.Unlock()
	got := c.DialCandidates(peer)
	if len(got) != 1 || got[0] != netip.MustParseAddr("10.1.1.1") {
		t.Fatalf("bad CIDR not skipped: %v", got)
	}
}

func TestDialCandidatesUnknownPeer(t *testing.T) {
	c := newTestCluster(t, id.New(), nil)
	if got := c.DialCandidates(id.New()); got != nil {
		t.Fatalf("unknown peer should yield nil, got %v", got)
	}
}
