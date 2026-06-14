// Package exec is the device adapter that pipes canonical PCM into an external
// player subprocess (pw-play / pw-cat / aplay / paplay). It is the portable,
// zero-config output: no libasound, no device enumeration — just a tool on $PATH.
//
// RATE PACING (the port contract, see device.go): the adapter does NOT synthesise
// a clock. Writing to the player's stdin pipe blocks on OS pipe backpressure the
// moment the downstream (pw-play → PipeWire → DAC) is full, and THAT backpressure
// is the engine's rate pacer. The blocking Write runs UNLOCKED (the handle is
// taken under the mutex, then written to without it) so Interrupt/Close can abort
// a parked write by closing the pipe from another goroutine.
//
// NO PHASE PROBE (deliberate): the player/PipeWire/DAC latency is opaque behind
// the pipe, so this adapter implements neither DelayReporter nor LatencyReporter.
// The engine therefore holds the resample ratio near 1 and leans on prime
// alignment plus the D36 calibration constant instead of a continuous phase lock.
package exec

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"ensemble/internal/sink/device"
	"ensemble/internal/stream"
)

// execTool is one player command in the auto-pick order (§8.5).
type execTool struct {
	name string
	args []string
}

// execTools is the auto-pick order. First found on $PATH wins.
var execTools = []execTool{
	{"pw-play", []string{"--raw", "--rate", "48000", "--channels", "2", "--format", "s16", "-"}},
	{"pw-cat", []string{"-p", "--raw", "--rate", "48000", "--channels", "2", "--format", "s16", "-"}},
	{"aplay", []string{"-q", "-f", "S16_LE", "-r", "48000", "-c", "2", "-t", "raw", "-"}},
	{"paplay", []string{"--raw", "--rate=48000", "--channels=2", "--format=s16le"}},
}

// lookExecTool returns the first execTool whose binary is on $PATH, or ok=false.
func lookExecTool() (execTool, string, bool) {
	for _, t := range execTools {
		if p, err := exec.LookPath(t.name); err == nil {
			return t, p, true
		}
	}
	return execTool{}, "", false
}

// lookExecToolNamed resolves a specific tool by name (for the resilient failover
// chain, which tries each player individually). ok=false if unknown or not on $PATH.
func lookExecToolNamed(name string) (execTool, string, bool) {
	for _, t := range execTools {
		if t.name != name {
			continue
		}
		if p, err := exec.LookPath(t.name); err == nil {
			return t, p, true
		}
		return execTool{}, "", false
	}
	return execTool{}, "", false
}

// childEnv returns the environment for a player subprocess. Under systemd as
// root there is no $HOME, which some players (notably the PipeWire/PulseAudio
// clients) need to locate per-user config; fall back to a writable temp dir so
// they don't abort. Everything else is inherited.
func childEnv() []string {
	env := os.Environ()
	if os.Getenv("HOME") == "" {
		env = append(env, "HOME="+os.TempDir())
	}
	return env
}

// respawnThrottle bounds how often a write failure may respawn the player, so a
// persistently-failing tool (e.g. no audio device) can't fork-bomb at frame
// cadence. The first failure of an episode respawns immediately (lastSpawn zero).
const respawnThrottle = time.Second

// sink pipes canonical PCM (s16le 48k stereo) into a player subprocess. It is the
// exec device adapter: Write blocks on pipe backpressure (the rate pacer), Flush
// respawns the player, Interrupt aborts a parked Write, and Close timeout-kills.
type sink struct {
	toolPath string
	toolName string
	toolArgs []string
	log      *slog.Logger

	once   sync.Once
	mu     sync.Mutex
	cmd    *exec.Cmd
	in     io.WriteCloser // stdin pipe; written to UNLOCKED so Interrupt/Close can abort
	closed bool

	noRespawn bool      // standalone self-heals; failover candidate surfaces deaths as write errors
	lastSpawn time.Time // last (re)spawn, for throttling respawn-on-write-failure

	// telemetry (StatsReporter); atomic so DeviceStats reads without the write lock.
	framesWritten atomic.Uint64
	writeErrors   atomic.Uint64
	respawns      atomic.Uint64 // reported as Underruns
}

// compile-time capability assertions: the exec adapter implements Sink plus the
// optional capabilities it can honour, and deliberately NOT Delay/LatencyReporter.
var (
	_ device.Sink          = (*sink)(nil)
	_ device.Flusher       = (*sink)(nil)
	_ device.Interrupter   = (*sink)(nil)
	_ device.StatsReporter = (*sink)(nil)
)

// open starts a player. toolName "" auto-picks the first on $PATH; otherwise it
// pins that specific tool. respawn controls internal self-heal: the resilient
// failover chain passes respawn=false so a dead player surfaces as a write error
// and the chain rotates to the next output (rather than respawning a player that
// just keeps dying — e.g. pw-play with no PipeWire session).
func open(toolName string, respawn bool, log *slog.Logger) (*sink, error) {
	var (
		tool execTool
		path string
		ok   bool
	)
	if toolName == "" {
		tool, path, ok = lookExecTool()
	} else {
		tool, path, ok = lookExecToolNamed(toolName)
	}
	if !ok {
		if toolName != "" {
			return nil, fmt.Errorf("exec backend: tool %q not available on $PATH", toolName)
		}
		return nil, fmt.Errorf("exec backend: no player tool on $PATH")
	}
	s := &sink{toolPath: path, toolName: tool.name, toolArgs: tool.args, log: log, noRespawn: !respawn}
	if err := s.spawnLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

// spawnLocked starts (or restarts) the player process. Caller holds s.mu (or is
// the constructor).
func (s *sink) spawnLocked() error {
	cmd := exec.Command(s.toolPath, s.toolArgs...)
	cmd.Env = childEnv()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("exec backend: stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("exec backend: start %s: %w", s.toolName, err)
	}
	s.cmd = cmd
	s.in = stdin
	s.log.Info("exec backend started", "tool", s.toolName)
	return nil
}

// Write plays one frame and BLOCKS on the player's stdin pipe backpressure (the
// rate pacer). The pipe handle is taken under the mutex then written UNLOCKED so a
// concurrent Interrupt/Close/Flush can close the pipe and abort a parked write.
func (s *sink) Write(frame []byte) error {
	if len(frame) != stream.FrameBytes {
		return fmt.Errorf("exec: frame %d bytes, want %d", len(frame), stream.FrameBytes)
	}
	// First attempt against the live pipe, unlocked (the player may apply
	// backpressure; holding s.mu across a blocking write would stall Close/Flush
	// and defeat Interrupt).
	s.mu.Lock()
	w := s.in
	s.mu.Unlock()
	if w != nil {
		if _, err := w.Write(frame); err == nil {
			s.framesWritten.Add(1)
			return nil
		}
	}
	// Write failed (broken pipe → the player process died) or no pipe.
	if s.noRespawn {
		// FAILOVER-CANDIDATE mode: the resilient chain owns retry, so surface the
		// death as a write error and let it rotate to the next output.
		s.writeErrors.Add(1)
		return fmt.Errorf("exec: %s player down", s.toolName)
	}
	// STANDALONE mode: respawn the player and retry once so playout self-heals
	// instead of going silent forever.
	w = s.respawn()
	if w == nil {
		s.writeErrors.Add(1)
		return fmt.Errorf("exec: player down")
	}
	if _, err := w.Write(frame); err != nil {
		s.writeErrors.Add(1)
		return err
	}
	s.framesWritten.Add(1)
	return nil
}

// respawn tears down the dead player and starts a fresh one, returning the new
// stdin pipe (or nil when throttled, closing, or the spawn failed). Throttled to
// respawnThrottle so a tool that dies on every start can't spin up processes at
// frame cadence. Also reaps the old (possibly self-exited) process, avoiding a
// zombie.
func (s *sink) respawn() io.WriteCloser {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	now := time.Now()
	if !s.lastSpawn.IsZero() && now.Sub(s.lastSpawn) < respawnThrottle {
		return nil // too soon: surface the error this frame, retry the next
	}
	s.lastSpawn = now
	if s.in != nil {
		_ = s.in.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
	}
	s.cmd, s.in = nil, nil
	s.respawns.Add(1)
	s.log.Warn("exec backend player died; respawning", "tool", s.toolName)
	if err := s.spawnLocked(); err != nil {
		s.log.Warn("exec backend respawn failed", "err", err)
		s.cmd, s.in = nil, nil
		return nil
	}
	return s.in
}

// Flush (device.Flusher) drops whatever the player buffered: pipe players retain
// queued audio across a write stall and replay it when writes resume — stale audio
// at the next session's start. The only reliable flush for an external process is
// a respawn. Counts as a respawn (Underruns) since it tears the player down.
func (s *sink) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return // shutting down: do not respawn a player (avoids a nil-pipe deref and
		//        zombie players after Close)
	}
	if s.in != nil {
		_ = s.in.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
	}
	s.cmd, s.in = nil, nil
	s.respawns.Add(1)
	if err := s.spawnLocked(); err != nil {
		s.log.Warn("exec backend respawn after flush failed", "err", err)
		s.cmd, s.in = nil, nil
	}
}

// Interrupt (device.Interrupter) aborts an in-flight blocking Write so Close/Reset
// stay snappy and a wedged player cannot deadlock shutdown. It closes the stdin
// pipe and kills the process: the parked, UNLOCKED Write is sitting in w.Write on
// that pipe, so closing it unblocks the write with an error immediately. It does
// NOT respawn — Interrupt is used on Close/Reset, where a fresh player is unwanted;
// in standalone mode a later Write will self-heal via respawn if the session lives on.
func (s *sink) Interrupt() {
	s.mu.Lock()
	in := s.in
	cmd := s.cmd
	s.in = nil
	s.cmd = nil
	s.mu.Unlock()
	if in != nil {
		_ = in.Close() // unblocks the parked Write
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
}

// Close (device.Sink) closes stdin, waits with a short timeout, and kills the
// process if it hangs (D21 — the exec backend gets a write deadline via process
// kill on Close). Idempotent.
func (s *sink) Close() error {
	var ret error
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true // stop any concurrent Flush/respawn from starting a new player
		in := s.in
		cmd := s.cmd
		s.in = nil
		s.cmd = nil
		s.mu.Unlock()
		if in != nil {
			_ = in.Close() // also unblocks any parked Write
		}
		if cmd == nil {
			return
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			ret = err
		case <-time.After(time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done
		}
	})
	return ret
}

// DeviceStats (device.StatsReporter) reports the exec adapter's telemetry. The
// player/PipeWire/DAC queue is opaque behind the pipe, so QueueNs/QueueValid and
// ConfiguredLatencyNs are zero/false (no phase probe). Underruns carries the player
// respawn count.
func (s *sink) DeviceStats() device.DeviceStats {
	return device.DeviceStats{
		Kind:                "exec",
		QueueNs:             0,
		QueueValid:          false,
		ConfiguredLatencyNs: 0,
		FramesWritten:       s.framesWritten.Load(),
		WriteErrors:         s.writeErrors.Load(),
		Underruns:           s.respawns.Load(),
	}
}
