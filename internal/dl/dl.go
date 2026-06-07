// Package dl loads optional shared libraries at runtime via purego (dlopen/
// dlsym FFI, no cgo, works with CGO_ENABLED=0). It is the single home for the
// "capability off" soft-fail (DECISIONS.md D32): a missing library, wrong
// version, or missing required symbol yields ErrUnavailable — never a panic —
// and the corresponding capability is simply reported off. Consumed by the
// opus codec (piece D) and the alsa backend (piece E).
package dl

import (
	"errors"
	"fmt"

	"github.com/ebitengine/purego"
)

// ErrUnavailable means the library could not be loaded or a required symbol was
// missing. Callers treat it as "capability off", not a fatal error.
var ErrUnavailable = errors.New("dl: shared library unavailable")

// Lib is a loaded shared library with all required symbols verified present.
type Lib struct {
	handle uintptr
	name   string
}

// Open tries each soname in order (e.g. "libopus.so.0", then "libopus.so") and,
// on the first that loads, dlsym-verifies EVERY symbol in `symbols` BEFORE
// returning. If no soname loads, or a loaded library is missing any required
// symbol, it returns ErrUnavailable (wrapped with context). Never panics.
func Open(sonames []string, symbols []string) (*Lib, error) {
	var lastErr error
	for _, name := range sonames {
		handle, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil || handle == 0 {
			lastErr = err
			continue
		}
		// Verify every required symbol resolves before we hand back the lib.
		if missing := verify(handle, symbols); missing != "" {
			purego.Dlclose(handle)
			return nil, fmt.Errorf("%w: %s missing symbol %q", ErrUnavailable, name, missing)
		}
		return &Lib{handle: handle, name: name}, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, lastErr)
	}
	return nil, fmt.Errorf("%w: none of %v loaded", ErrUnavailable, sonames)
}

// verify returns the name of the first symbol that fails to resolve, or "".
func verify(handle uintptr, symbols []string) string {
	for _, s := range symbols {
		ptr, err := purego.Dlsym(handle, s)
		if err != nil || ptr == 0 {
			return s
		}
	}
	return ""
}

// Name returns the soname that was actually loaded.
func (l *Lib) Name() string { return l.name }

// Func binds an exported C function to a Go func pointer (fptr must be a
// *func(...)). Panics like purego.RegisterLibFunc if fptr is the wrong shape —
// but the symbol is guaranteed present because Open verified the whole symbol
// list, so binding from that same list never fails here.
func (l *Lib) Func(fptr any, name string) {
	purego.RegisterLibFunc(fptr, l.handle, name)
}

// Close unloads the library. After Close the bound function pointers must not
// be called.
func (l *Lib) Close() error {
	if l.handle == 0 {
		return nil
	}
	err := purego.Dlclose(l.handle)
	l.handle = 0
	return err
}
