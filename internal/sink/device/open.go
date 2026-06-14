package device

import (
	"log/slog"
	"strings"
)

// Open resolves an ENSEMBLE_OUTPUT-style spec into a single Sink (D2/D27).
//
//	"" | "auto"      -> best available: alsa -> exec -> null (never errors)
//	"alsa"           -> alsa sink; errors if unregistered or device won't open
//	"exec"           -> first player on $PATH; degrades to null+WARN if none
//	"null"           -> null sink
//	"file:/abs/path" -> file sink appending raw PCM
//	"<name>[:arg]"   -> any registered factory, arg after the first colon
//
// Returns the sink and the resolved name (for /api/status + logging). This is the
// SINGLE-backend opener; the default output uses OpenResilient instead.
func Open(spec string, log *slog.Logger) (Sink, string, error) {
	return OpenDevice(spec, "", log)
}

// OpenDevice is Open with an explicit ALSA output device (D37, §8.5). The device is
// honored only on the alsa path (auto-selected alsa or an explicit "alsa" spec);
// every other backend ignores it (the exec backend in particular plays to its
// tool's own default — v1 limitation). An empty device means "default".
func OpenDevice(spec, device string, log *slog.Logger) (Sink, string, error) {
	if log == nil {
		log = slog.Default()
	}
	log = log.With("comp", "sink")

	name, arg := splitSpec(spec)

	switch name {
	case "", "auto":
		return openAuto(device, log)
	case "exec":
		b, err := openFactory("exec", "", log)
		if err != nil {
			log.Warn("exec backend unavailable, degrading to null", "err", err)
			nb, _ := openFactory("null", "", log)
			return nb, "null", nil
		}
		log.Info("backend selected", "backend", "exec", "reason", "explicit")
		return b, "exec", nil
	case "alsa":
		// Route the configured device through the factory arg.
		b, err := openFactory("alsa", device, log)
		if err != nil {
			return nil, "", err
		}
		log.Info("backend selected", "backend", "alsa", "reason", "explicit", "device", deviceLabel(device))
		return b, "alsa", nil
	}

	b, err := openFactory(name, arg, log)
	if err != nil {
		return nil, "", err
	}
	log.Info("backend selected", "backend", name, "reason", "explicit")
	return b, name, nil
}

// OpenResilient builds the self-healing output (the default for auto/alsa/exec): a
// failover chain over every real output on the host (the configured device first),
// rotating on failure and resting with exponential backoff after repeated
// whole-chain failures. The returned sink honors a device override (SetPreferred,
// via Playout.PreferOutputDevice) and a Revive hook (test tone). null/file specs
// bypass the chain and return that backend directly; a host with no real output
// returns null. Returns the sink and the resolved kind ("auto"/"null"/"file").
func OpenResilient(spec, device string, log *slog.Logger) (Sink, string, error) {
	if log == nil {
		log = slog.Default()
	}
	log = log.With("comp", "sink")

	switch name, _ := splitSpec(spec); name {
	case "null":
		b, _ := openFactory("null", "", log)
		log.Info("backend selected", "backend", "null", "reason", "explicit")
		return b, "null", nil
	case "file":
		b, _, err := OpenDevice(spec, device, log)
		return b, "file", err
	}

	// The preferred device for the chain is the explicit "alsa:<dev>" arg, else the
	// configured device (D37); the providers fall back to the system default.
	preferred := device
	if name, arg := splitSpec(spec); name == "alsa" && arg != "" {
		preferred = arg
	}

	cands := Candidates(preferred)
	if len(cands) == 0 {
		b, _ := openFactory("null", "", log)
		log.Info("backend selected", "backend", "null", "reason", "no real output device")
		return b, "null", nil
	}
	log.Info("resilient output chain", "candidates", candidateLabels(cands))
	return newResilientBackend(cands, log), "auto", nil
}

// openAuto picks the best single backend with no failover: alsa (if usable on this
// host) -> exec (if a player tool is present) -> null. Never errors.
func openAuto(device string, log *slog.Logger) (Sink, string, error) {
	if available("alsa") {
		if b, err := openFactory("alsa", device, log); err == nil {
			log.Info("backend selected", "backend", "alsa", "reason", "auto", "device", deviceLabel(device))
			return b, "alsa", nil
		} else {
			log.Warn("alsa registered but failed to open, trying exec", "err", err)
		}
	}
	if available("exec") {
		if b, err := openFactory("exec", "", log); err == nil {
			log.Info("backend selected", "backend", "exec", "reason", "auto")
			return b, "exec", nil
		} else {
			log.Warn("exec backend failed to open, falling back to null", "err", err)
		}
	}
	b, _ := openFactory("null", "", log)
	log.Info("backend selected", "backend", "null", "reason", "auto (no playback device)")
	return b, "null", nil
}

// candidateLabels renders the chain for a startup log line.
func candidateLabels(cands []Candidate) string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Label
	}
	return strings.Join(out, " → ")
}
