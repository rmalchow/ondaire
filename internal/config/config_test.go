package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// envMap returns a Getenv func backed by m.
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(Options{
		Args:   nil,
		Getenv: envMap(map[string]string{EnvDataDir: dir}),
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPPort != DefaultHTTPPort {
		t.Errorf("http = %d, want %d", cfg.HTTPPort, DefaultHTTPPort)
	}
	if cfg.StreamPort != DefaultStreamPort {
		t.Errorf("stream = %d, want %d", cfg.StreamPort, DefaultStreamPort)
	}
	if cfg.SourcePort != DefaultSourcePort {
		t.Errorf("source = %d, want %d", cfg.SourcePort, DefaultSourcePort)
	}
	if cfg.GossipPort != DefaultGossipPort {
		t.Errorf("gossip = %d, want %d", cfg.GossipPort, DefaultGossipPort)
	}
	wantData, _ := filepath.Abs(dir)
	if cfg.DataDir != wantData {
		t.Errorf("dataDir = %q, want %q", cfg.DataDir, wantData)
	}
	if cfg.MediaDir != filepath.Join(wantData, "media") {
		t.Errorf("mediaDir = %q, want %q", cfg.MediaDir, filepath.Join(wantData, "media"))
	}
	if cfg.Output != "" {
		t.Errorf("output = %q, want empty", cfg.Output)
	}
	if cfg.Join != nil {
		t.Errorf("join = %v, want nil", cfg.Join)
	}
}

func TestLoadFlagsOverrideEnv(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(Options{
		Args:   []string{"--http-port", "9000"},
		Getenv: envMap(map[string]string{EnvDataDir: dir, EnvHTTPPort: "1234"}),
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPPort != 9000 {
		t.Errorf("http = %d, want 9000 (flag wins)", cfg.HTTPPort)
	}
}

func TestLoadEnvFallback(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(Options{
		Getenv: envMap(map[string]string{EnvDataDir: dir, EnvStreamPort: "9100"}),
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.StreamPort != 9100 {
		t.Errorf("stream = %d, want 9100 (env)", cfg.StreamPort)
	}
}

func TestLoadSourcePortFlagAndEnv(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Load(Options{
		Args:   []string{"--source-port", "9300"},
		Getenv: envMap(map[string]string{EnvDataDir: dir, EnvSourcePort: "9250"}),
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SourcePort != 9300 {
		t.Errorf("source = %d, want 9300 (flag wins)", cfg.SourcePort)
	}

	cfg, err = Load(Options{
		Getenv: envMap(map[string]string{EnvDataDir: dir, EnvSourcePort: "9250"}),
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SourcePort != 9250 {
		t.Errorf("source = %d, want 9250 (env)", cfg.SourcePort)
	}

	cfg, err = Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir})})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SourcePort != DefaultSourcePort {
		t.Errorf("source = %d, want %d (default)", cfg.SourcePort, DefaultSourcePort)
	}
}

func TestLoadDataDirEnv(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir})})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want, _ := filepath.Abs(dir)
	if cfg.DataDir != want {
		t.Errorf("dataDir = %q, want %q", cfg.DataDir, want)
	}
	if _, err := os.Stat(filepath.Join(want, nodeFileName)); err != nil {
		t.Errorf("node.json not in data dir: %v", err)
	}
}

func TestLoadMediaDirDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir})})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(cfg.DataDir, "media")
	if cfg.MediaDir != want {
		t.Errorf("mediaDir = %q, want %q", cfg.MediaDir, want)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Errorf("media dir not created: %v", err)
	}
}

func TestLoadMediaDirEnvOverride(t *testing.T) {
	dir := t.TempDir()
	media := t.TempDir()
	cfg, err := Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir, EnvMediaDir: media})})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want, _ := filepath.Abs(media)
	if cfg.MediaDir != want {
		t.Errorf("mediaDir = %q, want %q", cfg.MediaDir, want)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Errorf("media dir not created: %v", err)
	}
}

func TestLoadDataDirMadeAbsolute(t *testing.T) {
	// A relative --data must resolve via filepath.Abs against CWD.
	dir := t.TempDir()
	rel := filepath.Join(dir, "sub")
	cfg, err := Load(Options{Args: []string{"--data", rel}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !filepath.IsAbs(cfg.DataDir) {
		t.Errorf("dataDir not absolute: %q", cfg.DataDir)
	}
}

func TestLoadOutputEnv(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir, EnvOutput: "null"})})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output != "null" {
		t.Errorf("output = %q, want null", cfg.Output)
	}

	cfg, err = Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir, EnvOutput: "file:/tmp/o.pcm"})})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output != "file:/tmp/o.pcm" {
		t.Errorf("output = %q, want verbatim file:/tmp/o.pcm", cfg.Output)
	}
}

func TestLoadJoinFlagSplits(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(Options{
		Args:   []string{"--join", "a:7946, b:7947 ,,"},
		Getenv: envMap(map[string]string{EnvDataDir: dir}),
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"a:7946", "b:7947"}
	if !reflect.DeepEqual(cfg.Join, want) {
		t.Errorf("join = %v, want %v", cfg.Join, want)
	}
}

func TestLoadJoinEnvFallback(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir, EnvJoin: "h:7946"})})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(cfg.Join, []string{"h:7946"}) {
		t.Errorf("join = %v, want [h:7946]", cfg.Join)
	}

	cfg, err = Load(Options{
		Args:   []string{"--join", "x:1"},
		Getenv: envMap(map[string]string{EnvDataDir: dir, EnvJoin: "h:7946"}),
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(cfg.Join, []string{"x:1"}) {
		t.Errorf("join = %v, want [x:1] (flag wins)", cfg.Join)
	}
}

func TestLoadJoinUnsetIsNil(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir})})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Join != nil {
		t.Errorf("join = %v, want nil", cfg.Join)
	}
}

func TestLoadNameFirstStartOnly(t *testing.T) {
	dir := t.TempDir()
	getenv := envMap(map[string]string{EnvDataDir: dir})

	cfg, err := Load(Options{Args: []string{"--name", "a"}, Getenv: getenv})
	if err != nil {
		t.Fatalf("first Load: %v", err)
	}
	if cfg.NodeName != "a" {
		t.Errorf("name = %q, want a", cfg.NodeName)
	}

	cfg, err = Load(Options{Args: []string{"--name", "b"}, Getenv: getenv})
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if cfg.NodeName != "a" {
		t.Errorf("name = %q, want a (first start only)", cfg.NodeName)
	}
}

func TestLoadBadPortEnv(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir, EnvHTTPPort: "abc"})})
	if err == nil {
		t.Error("want error for non-numeric port")
	}
}

func TestLoadPortOutOfRange(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(Options{
		Args:   []string{"--gossip-port", "70000"},
		Getenv: envMap(map[string]string{EnvDataDir: dir}),
	})
	if err == nil {
		t.Error("want error for out-of-range port")
	}
}

func TestLoadBadFlag(t *testing.T) {
	_, err := Load(Options{Args: []string{"--nope"}})
	if err == nil {
		t.Error("want error for unknown flag")
	}
}

func TestLoadUnwritableDataDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; MkdirAll over a file would not fail")
	}
	dir := t.TempDir()
	// Point --data at an existing regular file so MkdirAll fails.
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(Options{Args: []string{"--data", file}})
	if err == nil {
		t.Error("want error when data dir is a regular file")
	}
}

func TestConfigRenamePersistsAndUpdatesField(t *testing.T) {
	dir := t.TempDir()
	getenv := envMap(map[string]string{EnvDataDir: dir})
	cfg, err := Load(Options{Getenv: getenv})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.Rename("hall"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if cfg.NodeName != "hall" {
		t.Errorf("NodeName = %q, want hall", cfg.NodeName)
	}
	reloaded, err := Load(Options{Getenv: getenv})
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.NodeName != "hall" {
		t.Errorf("persisted name = %q, want hall", reloaded.NodeName)
	}
}

func TestConfigLoadDefaultsVolumeAndDelay(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir})})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Volume != 1.0 {
		t.Errorf("volume = %v, want 1.0", cfg.Volume)
	}
	if cfg.OutputDelayMs != 0 {
		t.Errorf("outputDelayMs = %v, want 0", cfg.OutputDelayMs)
	}
}

func TestConfigSetVolumePersistsAndUpdatesField(t *testing.T) {
	dir := t.TempDir()
	getenv := envMap(map[string]string{EnvDataDir: dir})
	cfg, err := Load(Options{Getenv: getenv})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.SetVolume(0.6); err != nil {
		t.Fatalf("SetVolume: %v", err)
	}
	if cfg.Volume != 0.6 {
		t.Errorf("Volume = %v, want 0.6", cfg.Volume)
	}
	reloaded, _ := Load(Options{Getenv: getenv})
	if reloaded.Volume != 0.6 {
		t.Errorf("persisted volume = %v, want 0.6", reloaded.Volume)
	}
	if err := cfg.SetVolume(2.0); err != nil {
		t.Fatalf("SetVolume clamp: %v", err)
	}
	if cfg.Volume != 1.0 {
		t.Errorf("Volume = %v, want 1.0 (clamped)", cfg.Volume)
	}
}

func TestConfigSetOutputDelayMsPersistsAndUpdatesField(t *testing.T) {
	dir := t.TempDir()
	getenv := envMap(map[string]string{EnvDataDir: dir})
	cfg, err := Load(Options{Getenv: getenv})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.SetOutputDelayMs(150); err != nil {
		t.Fatalf("SetOutputDelayMs: %v", err)
	}
	if cfg.OutputDelayMs != 150 {
		t.Errorf("OutputDelayMs = %v, want 150", cfg.OutputDelayMs)
	}
	reloaded, _ := Load(Options{Getenv: getenv})
	if reloaded.OutputDelayMs != 150 {
		t.Errorf("persisted delay = %v, want 150", reloaded.OutputDelayMs)
	}
	if err := cfg.SetOutputDelayMs(9000); err != nil {
		t.Fatalf("SetOutputDelayMs clamp: %v", err)
	}
	if cfg.OutputDelayMs != 500 {
		t.Errorf("OutputDelayMs = %v, want 500 (clamped)", cfg.OutputDelayMs)
	}
}

func TestNodeFilePath(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(Options{Getenv: envMap(map[string]string{EnvDataDir: dir})})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(cfg.DataDir, nodeFileName)
	if cfg.NodeFilePath() != want {
		t.Errorf("NodeFilePath = %q, want %q", cfg.NodeFilePath(), want)
	}
}
