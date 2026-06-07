package dl

import (
	"errors"
	"runtime"
	"testing"
)

// libc carries common, stable symbols on every Linux libc.
const libcSoname = "libc.so.6"

func skipNonLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("dl tests target linux libc; GOOS=%s", runtime.GOOS)
	}
}

func TestOpenLibcSucceeds(t *testing.T) {
	skipNonLinux(t)
	lib, err := Open([]string{libcSoname, "libc.so"}, []string{"strlen", "memcpy"})
	if err != nil {
		t.Fatalf("Open(libc): %v", err)
	}
	defer lib.Close()
	if lib.Name() == "" {
		t.Fatal("loaded lib has empty name")
	}
}

func TestOpenBogusSonameUnavailable(t *testing.T) {
	_, err := Open([]string{"libdefinitelynotreal_ensemble.so.999"}, []string{"foo"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestOpenBogusSymbolUnavailableNoPanic(t *testing.T) {
	skipNonLinux(t)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Open panicked on missing symbol: %v", r)
		}
	}()
	_, err := Open([]string{libcSoname, "libc.so"},
		[]string{"strlen", "this_symbol_does_not_exist_in_libc_xyz"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestFuncBindsCallable(t *testing.T) {
	skipNonLinux(t)
	lib, err := Open([]string{libcSoname, "libc.so"}, []string{"strlen"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer lib.Close()
	var strlen func(s string) uint64
	lib.Func(&strlen, "strlen")
	if got := strlen("hello\x00"); got != 5 {
		t.Fatalf("strlen(\"hello\") = %d, want 5", got)
	}
}
