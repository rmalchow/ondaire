package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"ondaire/internal/api"
	"ondaire/internal/contracts"
	"ondaire/internal/group"
	"ondaire/internal/stream"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseOptionsDefaults(t *testing.T) {
	opt, err := parseOptions(nil, env(nil))
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if opt.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", opt.LogLevel)
	}
	if opt.Output != "" {
		t.Errorf("Output = %q, want empty (auto)", opt.Output)
	}
	if opt.Host != "" {
		t.Errorf("Host = %q, want empty", opt.Host)
	}
}

func TestParseOptionsOutputNull(t *testing.T) {
	opt, err := parseOptions(nil, env(map[string]string{"ONDAIRE_OUTPUT": "null"}))
	if err != nil {
		t.Fatal(err)
	}
	if opt.Output != "null" {
		t.Errorf("Output = %q, want null", opt.Output)
	}
}

func TestParseOptionsOutputFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--output", "null"},
		{"--output=null"},
	} {
		opt, err := parseOptions(args, env(nil))
		if err != nil {
			t.Fatalf("args %v: %v", args, err)
		}
		if opt.Output != "null" {
			t.Errorf("args %v: Output = %q, want null", args, opt.Output)
		}
		// --output must be stripped from the args forwarded to config.Load.
		for _, a := range opt.cfgArgs {
			if a == "--output" || a == "null" || a == "--output=null" {
				t.Errorf("args %v: --output leaked into cfgArgs: %v", args, opt.cfgArgs)
			}
		}
	}
}

// The flag overrides the ONDAIRE_OUTPUT env fallback (flag > env > default).
func TestParseOptionsOutputFlagBeatsEnv(t *testing.T) {
	opt, err := parseOptions([]string{"--output", "alsa"}, env(map[string]string{"ONDAIRE_OUTPUT": "null"}))
	if err != nil {
		t.Fatal(err)
	}
	if opt.Output != "alsa" {
		t.Errorf("Output = %q, want alsa (flag beats env)", opt.Output)
	}
}

func TestParseOptionsOutputMissingArg(t *testing.T) {
	if _, err := parseOptions([]string{"--output"}, env(nil)); err == nil {
		t.Fatal("expected error for --output without value")
	}
}

func TestParseOptionsLogLevel(t *testing.T) {
	opt, _ := parseOptions(nil, env(map[string]string{"ONDAIRE_LOG": "debug"}))
	if opt.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", opt.LogLevel)
	}
}

func TestParseOptionsHostFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--host", "127.0.0.1"},
		{"--host=127.0.0.1"},
	} {
		opt, err := parseOptions(args, env(nil))
		if err != nil {
			t.Fatalf("args %v: %v", args, err)
		}
		if opt.Host != "127.0.0.1" {
			t.Errorf("args %v: Host = %q, want 127.0.0.1", args, opt.Host)
		}
		// --host must be stripped from the args forwarded to config.Load.
		for _, a := range opt.cfgArgs {
			if a == "--host" || a == "127.0.0.1" || a == "--host=127.0.0.1" {
				t.Errorf("args %v: --host leaked into cfgArgs: %v", args, opt.cfgArgs)
			}
		}
	}
}

// validateConfigFlags (the up-front dry-parse) must accept every flag config.Load
// defines, or a real flag is rejected before config ever sees it. This guards the
// two flag lists against drift (the --role/--control-port regression).
func TestParseOptionsAcceptsConfigFlags(t *testing.T) {
	args := []string{
		"--role", "master",
		"--control-port", "9300",
		"--http-port", "8080",
		"--stream-port", "9090",
		"--source-port", "9200",
		"--gossip-port", "7946",
		"--data", "/tmp/x",
		"--no-mdns",
	}
	if _, err := parseOptions(args, env(nil)); err != nil {
		t.Fatalf("parseOptions rejected a valid config flag: %v", err)
	}
}

func TestParseOptionsHostEnv(t *testing.T) {
	opt, _ := parseOptions(nil, env(map[string]string{"ONDAIRE_HOST": "127.0.0.1"}))
	if opt.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", opt.Host)
	}
}

func TestParseOptionsForwardsConfigFlags(t *testing.T) {
	opt, err := parseOptions([]string{"--http-port", "18080", "--host", "127.0.0.1", "--name", "n1"}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"--http-port": true, "18080": true, "--name": true, "n1": true}
	got := map[string]bool{}
	for _, a := range opt.cfgArgs {
		got[a] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("cfgArgs missing %q: %v", w, opt.cfgArgs)
		}
	}
}

func TestParseOptionsBadPort(t *testing.T) {
	if _, err := parseOptions([]string{"--http-port", "notanumber"}, env(nil)); err == nil {
		t.Fatal("expected parse error for non-numeric --http-port")
	}
}

func TestParseOptionsHostMissingArg(t *testing.T) {
	if _, err := parseOptions([]string{"--host"}, env(nil)); err == nil {
		t.Fatal("expected error for --host without value")
	}
}

func TestForcedNull(t *testing.T) {
	if !forcedNull("null") {
		t.Error("forcedNull(null) = false")
	}
	if forcedNull("auto") || forcedNull("") || forcedNull("alsa") {
		t.Error("forcedNull true for a non-null spec")
	}
}

func TestCapabilitiesNullForcesNoPlayback(t *testing.T) {
	caps := capabilities(options{Output: "null"})
	if caps.Playback {
		t.Error("Playback = true with forced null")
	}
	if !contains(caps.Sources, "file") || !contains(caps.Sources, "http") {
		t.Errorf("Sources missing file/http: %v", caps.Sources)
	}
	if !contains(caps.Codecs, "pcm") {
		t.Errorf("Codecs missing pcm: %v", caps.Codecs)
	}
	if !contains(caps.Formats, "wav") {
		t.Errorf("Formats missing wav: %v", caps.Formats)
	}
}

func TestCapabilitiesBackendsAlwaysHaveNullAndFile(t *testing.T) {
	caps := capabilities(options{Output: "null"})
	if !contains(caps.Backends, "null") || !contains(caps.Backends, "file") {
		t.Errorf("Backends missing null/file: %v", caps.Backends)
	}
}

func TestHostCIDR(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1": "127.0.0.1/32",
		"::1":       "::1/128",
		"":          "",
		"0.0.0.0":   "",
		"::":        "",
		"garbage":   "",
	}
	for in, want := range cases {
		if got := hostCIDR(in); got != want {
			t.Errorf("hostCIDR(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShutdownStackLIFO(t *testing.T) {
	var order []string
	s := &shutdownStack{}
	s.push("a", func(context.Context) error { order = append(order, "a"); return nil })
	s.push("b", func(context.Context) error { order = append(order, "b"); return errors.New("boom") })
	s.push("c", func(context.Context) error { order = append(order, "c"); return nil })

	err := s.unwind(context.Background(), newLogger("error"))
	if err == nil || err.Error() != "boom" {
		t.Errorf("unwind returned %v, want boom", err)
	}
	want := []string{"c", "b", "a"}
	if len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Errorf("order = %v, want %v (all run, reverse)", order, want)
	}
}

func TestProbeGossipPortReleases(t *testing.T) {
	port, released, err := probeGossipPort("127.0.0.1", 17946, 64)
	if err != nil {
		t.Fatalf("probeGossipPort: %v", err)
	}
	if !released {
		t.Fatal("probeGossipPort did not report release")
	}
	// Both TCP and UDP must be immediately re-bindable (proves probe-release, D8).
	tcp, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		t.Fatalf("TCP not re-bindable on %d: %v", port, err)
	}
	tcp.Close()
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		t.Fatalf("UDP not re-bindable on %d: %v", port, err)
	}
	udp.Close()
}

func TestMapErrTranslatesToAPISentinels(t *testing.T) {
	cases := []struct {
		in   error
		want error
	}{
		{group.ErrNoOpus, api.ErrNoCodec},
		{group.ErrBadSettings, api.ErrNoCodec},
		{group.ErrNotSynced, api.ErrNotSynced},
	}
	for _, c := range cases {
		got := mapErr(c.in)
		if !errors.Is(got, c.want) {
			t.Errorf("mapErr(%v): errors.Is(_, %v) = false", c.in, c.want)
		}
		// Original message must be preserved.
		if got.Error() != c.in.Error() {
			t.Errorf("mapErr(%v).Error() = %q, want %q", c.in, got.Error(), c.in.Error())
		}
	}
	if mapErr(nil) != nil {
		t.Error("mapErr(nil) != nil")
	}
}

func TestMapErrPreservesOpusNodeNames(t *testing.T) {
	// Simulate the engine's wrapped opus error (Play wraps ErrNoOpus with names).
	wrapped := fmt.Errorf("%w: alice, bob", group.ErrNoOpus)
	got := mapErr(wrapped)
	if !errors.Is(got, api.ErrNoCodec) {
		t.Fatal("wrapped opus error not mapped to ErrNoCodec")
	}
	if got.Error() != wrapped.Error() {
		t.Errorf("message = %q, want %q", got.Error(), wrapped.Error())
	}
}

// fakeSink records the gens it was Reset to and the frames pushed, for the
// deliver-callback test.
type fakeSink struct {
	resets []uint32
	pushes []uint32
}

func (f *fakeSink) Push(gen uint32, seq uint64, pts int64, payload []byte) {
	f.pushes = append(f.pushes, gen)
}
func (f *fakeSink) Reset(gen uint32)           { f.resets = append(f.resets, gen) }
func (f *fakeSink) Disarm()                    {}
func (f *fakeSink) Stats() contracts.SinkStats { return contracts.SinkStats{} }
func (f *fakeSink) SetGain(float64)            {}
func (f *fakeSink) SetDelayOffset(int64)       {}
func (f *fakeSink) Close() error               { return nil }

func TestNewDeliverPCMResetsOnGenChange(t *testing.T) {
	fs := &fakeSink{}
	d := newDeliver(fs, newDisableState(nil), newLogger("error"))
	pcm := make([]byte, stream.FrameBytes)

	d(stream.Header{Gen: 7, Seq: 0, PTS: 0}, pcm)
	d(stream.Header{Gen: 7, Seq: 1, PTS: 0}, pcm)
	d(stream.Header{Gen: 9, Seq: 0, PTS: 0}, pcm) // gen change → re-arm

	if len(fs.resets) != 2 || fs.resets[0] != 7 || fs.resets[1] != 9 {
		t.Errorf("resets = %v, want [7 9]", fs.resets)
	}
	if len(fs.pushes) != 3 {
		t.Errorf("pushes = %d, want 3", len(fs.pushes))
	}
	for _, g := range fs.pushes {
		if g != 7 && g != 9 {
			t.Errorf("unexpected push gen %d", g)
		}
	}
}

// genSink is a fakeSink that also tracks its armed gen (implements ArmedGen),
// modelling the real *sink.Playout. It lets the deliver test reproduce the
// late-join stale-gen bug: when something Resets the sink behind deliver's back
// to a gen that does NOT match the incoming frames, deliver must re-arm.
type genSink struct {
	resets []uint32
	pushes []uint32
	gen    uint32
	armed  bool
}

func (g *genSink) Push(gen uint32, seq uint64, pts int64, payload []byte) {
	g.pushes = append(g.pushes, gen)
}
func (g *genSink) Reset(gen uint32)           { g.resets = append(g.resets, gen); g.gen = gen; g.armed = true }
func (g *genSink) Disarm()                    { g.armed = false }
func (g *genSink) Stats() contracts.SinkStats { return contracts.SinkStats{} }
func (g *genSink) SetGain(float64)            {}
func (g *genSink) SetDelayOffset(int64)       {}
func (g *genSink) Close() error               { return nil }
func (g *genSink) ArmedGen() (uint32, bool)   { return g.gen, g.armed }

// TestNewDeliverReArmsOnSinkGenMismatch reproduces the late-join bug: the group
// engine (repointLocked) Resets the sink to a GUESSED gen (0) on a (re)subscribe
// while deliver's cached curGen still equals the incoming frame gen. Deliver must
// notice the sink is armed at the wrong gen and re-arm to the frame's gen — else
// every frame drops as stale-gen and the joiner starves.
func TestNewDeliverReArmsOnSinkGenMismatch(t *testing.T) {
	gs := &genSink{}
	d := newDeliver(gs, newDisableState(nil), newLogger("error"))
	pcm := make([]byte, stream.FrameBytes)

	// First subscription: frames at gen 1 arm the sink to 1.
	d(stream.Header{Gen: 1, Seq: 0, PTS: 0}, pcm)
	if !gs.armed || gs.gen != 1 {
		t.Fatalf("after first frame: gen=%d armed=%v want 1 true", gs.gen, gs.armed)
	}

	// repointLocked re-subscribes and Resets the sink to the guessed gen 0
	// (member floor). Deliver's cache still says curGen=1.
	gs.Reset(0)

	// New frames still arrive at gen 1 (the master's real, unchanged gen).
	d(stream.Header{Gen: 1, Seq: 1, PTS: 0}, pcm)

	// Deliver must have re-armed the sink to gen 1 (not left it at 0).
	if gs.gen != 1 {
		t.Errorf("sink gen = %d, want 1 (deliver did not re-arm after external Reset to 0)", gs.gen)
	}
	// resets: [1 (first frame), 0 (repoint), 1 (deliver re-arm)].
	if len(gs.resets) != 3 || gs.resets[2] != 1 {
		t.Errorf("resets = %v, want last reset to 1", gs.resets)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
