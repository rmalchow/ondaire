package device

import (
	"log/slog"
	"sort"
	"strings"
	"sync"

	"ondaire/internal/contracts"
)

// Factory builds a device from its spec argument (the part after the colon in
// ONDAIRE_OUTPUT, e.g. the path for "file:/tmp/x", "" for "null"/"exec"). Each
// adapter registers one from its package init().
type Factory func(arg string, log *slog.Logger) (Sink, error)

// Enumerator lists the host's output devices for one backend kind (alsa parses
// /proc/asound/pcm; others contribute none). Registered by the adapter so this
// package never imports the adapters — keeping it a near-leaf.
type Enumerator func() []contracts.OutputDevice

// reg is one registered backend kind.
type reg struct {
	factory   Factory
	available func() bool       // host can actually use it now (libasound loaded, tool on $PATH); nil ⇒ always
	enum      Enumerator        // optional device enumerator (UI device list)
	cands     CandidateProvider // optional failover-chain candidate provider
}

var (
	regMu    sync.RWMutex
	registry = map[string]*reg{}
)

func entry(name string) *reg {
	if r := registry[name]; r != nil {
		return r
	}
	r := &reg{}
	registry[name] = r
	return r
}

// Register adds a named device factory. `available` reports whether the host can
// use this backend right now (nil ⇒ always usable, e.g. null/file). Called from
// each adapter's init(); registering a name twice panics (programmer error).
func Register(name string, f Factory, available func() bool) {
	regMu.Lock()
	defer regMu.Unlock()
	r := entry(name)
	if r.factory != nil {
		panic("device: backend already registered: " + name)
	}
	r.factory = f
	r.available = available
}

// RegisterEnumerator attaches an output-device enumerator to a backend kind.
func RegisterEnumerator(name string, e Enumerator) {
	regMu.Lock()
	defer regMu.Unlock()
	entry(name).enum = e
}

// isRegistered reports whether a backend kind has a factory.
func isRegistered(name string) bool {
	regMu.RLock()
	defer regMu.RUnlock()
	r := registry[name]
	return r != nil && r.factory != nil
}

// available reports whether a registered backend can be used on this host now.
func available(name string) bool {
	regMu.RLock()
	defer regMu.RUnlock()
	r := registry[name]
	if r == nil || r.factory == nil {
		return false
	}
	return r.available == nil || r.available()
}

// BackendNames returns the registered backend kinds, sorted (capabilities.backends,
// §1/D3). Pure; no process spawn.
func BackendNames() []string {
	regMu.RLock()
	names := make([]string, 0, len(registry))
	for n, r := range registry {
		if r.factory != nil {
			names = append(names, n)
		}
	}
	regMu.RUnlock()
	sort.Strings(names)
	return names
}

// HasPlayback reports whether a real (non-null/file) output is usable on this host
// — drives capabilities.playback (§1, D27). Pure lookup; no spawn.
func HasPlayback() bool {
	regMu.RLock()
	defer regMu.RUnlock()
	for name, r := range registry {
		if name == "null" || name == "file" || r.factory == nil {
			continue
		}
		if r.available == nil || r.available() {
			return true
		}
	}
	return false
}

// ListOutputDevices returns every enumerated output device across all backends that
// registered an enumerator (D37, §8.5). Empty when none do (e.g. libasound absent).
func ListOutputDevices() []contracts.OutputDevice {
	regMu.RLock()
	enums := make([]Enumerator, 0, len(registry))
	for _, r := range registry {
		if r.enum != nil {
			enums = append(enums, r.enum)
		}
	}
	regMu.RUnlock()
	var out []contracts.OutputDevice
	for _, e := range enums {
		out = append(out, e()...)
	}
	return out
}

// Candidate is one openable output in the failover chain — a specific ALSA device
// or a specific exec tool. Real backends register a provider so the resilient
// backend stays backend-agnostic: a future output joins the chain by registering
// its own candidates, with no edit to the failover logic.
type Candidate struct {
	Kind  string // backend kind to open via the factory: "alsa" | "exec" | …
	Arg   string // factory arg: ALSA device id ("default"/"hw:0,0"), or exec tool name
	Label string // human label for logs / the UI
}

// Open constructs the backend for this candidate.
func (c Candidate) Open(log *slog.Logger) (Sink, error) { return openFactory(c.Kind, c.Arg, log) }

// CandidateProvider yields a backend's failover candidates, preferred device first.
// `preferred` is the operator/UI-selected device (D37), honoured by the alsa provider.
type CandidateProvider func(preferred string) []Candidate

// RegisterCandidates attaches a failover-candidate provider to a backend kind.
func RegisterCandidates(name string, p CandidateProvider) {
	regMu.Lock()
	defer regMu.Unlock()
	entry(name).cands = p
}

// Candidates assembles the host's full failover chain: every registered provider's
// candidates, deduped stable (preferred first). The resilient backend consumes this.
//
// Providers are walked in candidatePriority order (ALSA first), NOT registry-map
// order: ranging the map gave Go's randomized iteration order, so on a desktop where
// exec also offers candidates (pw-play/paplay on $PATH) the exec provider could be
// walked first and shadow a perfectly good ALSA output. ALSA is always the first
// choice — it is the only backend with a real device clock for the rate servo.
func Candidates(preferred string) []Candidate {
	regMu.RLock()
	type namedProvider struct {
		name string
		p    CandidateProvider
	}
	provs := make([]namedProvider, 0, len(registry))
	for n, r := range registry {
		if r.cands != nil {
			provs = append(provs, namedProvider{n, r.cands})
		}
	}
	regMu.RUnlock()
	sort.SliceStable(provs, func(i, j int) bool {
		if pi, pj := candidatePriority(provs[i].name), candidatePriority(provs[j].name); pi != pj {
			return pi < pj
		}
		return provs[i].name < provs[j].name
	})
	var out []Candidate
	seen := map[string]bool{}
	for _, np := range provs {
		for _, c := range np.p(preferred) {
			key := c.Kind + "|" + c.Arg
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, c)
		}
	}
	return out
}

// candidatePriority ranks backend kinds for the failover chain (lower = tried
// first): ALSA leads unconditionally, everything else follows in name order. This is
// what makes the chain deterministic and keeps "alsa → exec → null" honest.
func candidatePriority(name string) int {
	if name == "alsa" {
		return 0
	}
	return 1
}

// splitSpec splits "name:arg" on the first colon. "name" alone ⇒ arg "".
func splitSpec(spec string) (name, arg string) {
	spec = strings.TrimSpace(spec)
	if i := strings.IndexByte(spec, ':'); i >= 0 {
		return spec[:i], spec[i+1:]
	}
	return spec, ""
}

// deviceLabel renders a configured ALSA device for a log line ("default" when empty).
func deviceLabel(dev string) string {
	if dev == "" {
		return "default"
	}
	return dev
}

// openFactory constructs a registered backend by name.
func openFactory(name, arg string, log *slog.Logger) (Sink, error) {
	regMu.RLock()
	r := registry[name]
	regMu.RUnlock()
	if r == nil || r.factory == nil {
		return nil, &NotRegisteredError{Name: name}
	}
	return r.factory(arg, log)
}

// NotRegisteredError is returned when a spec names an unregistered backend.
type NotRegisteredError struct{ Name string }

func (e *NotRegisteredError) Error() string { return "device: backend " + e.Name + " not registered" }
