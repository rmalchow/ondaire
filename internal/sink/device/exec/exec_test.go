package exec

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"ondaire/internal/sink/device"
)

// compile-time capability assertions: exec honours Sink + Flusher + Interrupter +
// StatsReporter and deliberately NOT Delay/LatencyReporter (opaque pipe latency).
var (
	_ device.Sink          = (*sink)(nil)
	_ device.Flusher       = (*sink)(nil)
	_ device.Interrupter   = (*sink)(nil)
	_ device.StatsReporter = (*sink)(nil)
)

// fakePath builds a temp dir containing dummy executables for each named tool and
// points $PATH exclusively at it (via t.Setenv, auto-restored). The dummies just
// read stdin to EOF — enough to be exec.LookPath-resolvable and Start-able without
// being a real audio player. Returns the dir.
func fakePath(t *testing.T, tools ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("dummy-executable PATH fixture is POSIX-only")
	}
	dir := t.TempDir()
	const script = "#!/bin/sh\ncat >/dev/null\n"
	for _, name := range tools {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
			t.Fatalf("write dummy %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

func TestLookExecToolPrefersOrder(t *testing.T) {
	// Provide aplay and pw-play; pw-play is earlier in execTools so it must win.
	fakePath(t, "aplay", "pw-play")
	tool, path, ok := lookExecTool()
	if !ok {
		t.Fatal("expected a tool to resolve on the fake PATH")
	}
	if tool.name != "pw-play" {
		t.Fatalf("auto-pick=%q, want pw-play (preference order)", tool.name)
	}
	if filepath.Base(path) != "pw-play" {
		t.Fatalf("resolved path %q does not point at pw-play", path)
	}
}

func TestLookExecToolNoneOnEmptyPath(t *testing.T) {
	t.Setenv("PATH", "")
	if _, _, ok := lookExecTool(); ok {
		t.Fatal("no tool should resolve on an empty PATH")
	}
	if available() {
		t.Fatal("available() must be false with no tool on PATH")
	}
}

func TestLookExecToolNamed(t *testing.T) {
	fakePath(t, "aplay") // only aplay present
	if _, _, ok := lookExecToolNamed("aplay"); !ok {
		t.Fatal("aplay should resolve")
	}
	// Present in the known set but absent from PATH ⇒ not ok.
	if _, _, ok := lookExecToolNamed("pw-play"); ok {
		t.Fatal("pw-play is not on PATH; should not resolve")
	}
	// Unknown tool name (not in execTools) ⇒ not ok.
	if _, _, ok := lookExecToolNamed("totally-not-a-player"); ok {
		t.Fatal("unknown tool name should not resolve")
	}
}

func TestAvailableTrueWithTool(t *testing.T) {
	fakePath(t, "paplay")
	if !available() {
		t.Fatal("available() should be true when a player tool is on PATH")
	}
}

// TestCandidatesOnePerPresentTool: the provider emits one candidate per tool
// actually present, in execTools preference order, all Kind "exec" with the tool
// name as Arg. The preferred argument is ignored (exec has no selectable device).
func TestCandidatesOnePerPresentTool(t *testing.T) {
	fakePath(t, "paplay", "pw-play") // present out of order on disk
	got := candidates("ignored-preferred")

	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2: %+v", len(got), got)
	}
	// Preference order: pw-play (index 0 in execTools) before paplay (index 3).
	if got[0].Arg != "pw-play" || got[1].Arg != "paplay" {
		t.Fatalf("candidate order=[%q,%q], want [pw-play paplay]", got[0].Arg, got[1].Arg)
	}
	for _, c := range got {
		if c.Kind != "exec" {
			t.Errorf("candidate %q Kind=%q, want exec", c.Arg, c.Kind)
		}
		if c.Label != "exec("+c.Arg+")" {
			t.Errorf("candidate %q Label=%q, want exec(%s)", c.Arg, c.Label, c.Arg)
		}
	}
}

func TestCandidatesEmptyWhenNoTool(t *testing.T) {
	t.Setenv("PATH", "")
	if got := candidates(""); got != nil {
		t.Fatalf("no tools on PATH should yield no candidates, got %+v", got)
	}
}

// TestFactoryStandaloneAutoPicksAndSelfHeals: arg "" → auto-pick the first tool
// and run in STANDALONE mode (internal respawn ON ⇒ noRespawn == false).
func TestFactoryStandaloneAutoPicks(t *testing.T) {
	fakePath(t, "aplay", "pw-play")
	b, err := factory("", slog.Default())
	if err != nil {
		t.Fatalf("standalone factory: %v", err)
	}
	defer b.Close()
	s := b.(*sink)
	if s.toolName != "pw-play" {
		t.Fatalf("standalone auto-pick toolName=%q, want pw-play", s.toolName)
	}
	if s.noRespawn {
		t.Fatal("standalone mode must self-heal: noRespawn should be false")
	}
}

// TestFactoryPinnedCandidate: arg "<tool>" → pin that tool in FAILOVER-CANDIDATE
// mode (no internal respawn ⇒ noRespawn == true) so a death surfaces as a write
// error for the resilient chain to rotate on.
func TestFactoryPinnedCandidate(t *testing.T) {
	fakePath(t, "aplay", "pw-play")
	b, err := factory("aplay", slog.Default()) // pin the non-preferred tool
	if err != nil {
		t.Fatalf("pinned factory: %v", err)
	}
	defer b.Close()
	s := b.(*sink)
	if s.toolName != "aplay" {
		t.Fatalf("pinned toolName=%q, want aplay", s.toolName)
	}
	if !s.noRespawn {
		t.Fatal("pinned candidate mode must NOT self-heal: noRespawn should be true")
	}
}

func TestFactoryPinnedUnknownToolErrors(t *testing.T) {
	fakePath(t, "pw-play") // pw-play present, aplay absent
	if _, err := factory("aplay", slog.Default()); err == nil {
		t.Fatal("pinning a tool absent from PATH should error")
	}
}

func TestFactoryStandaloneNoToolErrors(t *testing.T) {
	t.Setenv("PATH", "")
	if _, err := factory("", slog.Default()); err == nil {
		t.Fatal("standalone factory with no tool on PATH should error")
	}
}

func TestExecDeviceStatsKind(t *testing.T) {
	fakePath(t, "pw-play")
	b, err := factory("", slog.Default())
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	defer b.Close()
	st := b.(*sink).DeviceStats()
	if st.Kind != "exec" {
		t.Fatalf("Kind=%q, want exec", st.Kind)
	}
	if st.QueueValid {
		t.Fatal("exec queue is opaque: QueueValid must be false")
	}
}
