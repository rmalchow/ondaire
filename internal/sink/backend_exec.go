package sink

import (
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"ensemble/internal/stream"
)

// execTool is one player command in the auto-pick order (§8.5).
type execTool struct {
	name string
	args []string
}

// execTools is the auto-pick order. First found on $PATH wins.
var execTools = []execTool{
	{"pw-play", []string{"--rate", "48000", "--channels", "2", "--format", "s16", "-"}},
	{"pw-cat", []string{"-p", "--rate", "48000", "--channels", "2", "--format", "s16", "-"}},
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

// execBackend pipes canonical PCM (s16le 48k stereo) into a player subprocess.
type execBackend struct {
	cmd  *exec.Cmd
	in   io.WriteCloser // stdin pipe
	log  *slog.Logger
	once sync.Once
	mu   sync.Mutex
}

func newExecBackend(log *slog.Logger) (*execBackend, error) {
	tool, path, ok := lookExecTool()
	if !ok {
		return nil, fmt.Errorf("exec backend: no player tool on $PATH")
	}
	cmd := exec.Command(path, tool.args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("exec backend: stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("exec backend: start %s: %w", tool.name, err)
	}
	log.Info("exec backend started", "tool", tool.name)
	return &execBackend{cmd: cmd, in: stdin, log: log}, nil
}

func (b *execBackend) Write(frame []byte) error {
	if len(frame) != stream.FrameBytes {
		return fmt.Errorf("exec: frame %d bytes, want %d", len(frame), stream.FrameBytes)
	}
	b.mu.Lock()
	w := b.in
	b.mu.Unlock()
	if w == nil {
		return fmt.Errorf("exec: closed")
	}
	_, err := w.Write(frame)
	return err
}

// Close closes stdin, waits with a short timeout, and kills the process if it
// hangs (D21 — exec backend gets a write deadline via process kill on Close).
func (b *execBackend) Close() error {
	var ret error
	b.once.Do(func() {
		b.mu.Lock()
		in := b.in
		b.in = nil
		b.mu.Unlock()
		if in != nil {
			_ = in.Close()
		}
		done := make(chan error, 1)
		go func() { done <- b.cmd.Wait() }()
		select {
		case err := <-done:
			ret = err
		case <-time.After(time.Second):
			_ = b.cmd.Process.Kill()
			<-done
		}
	})
	return ret
}
