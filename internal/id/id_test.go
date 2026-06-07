package id

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewRandomDistinct(t *testing.T) {
	a, b := New(), New()
	if a == b {
		t.Fatal("two New() returned identical IDs")
	}
	if a.IsZero() || b.IsZero() {
		t.Fatal("New() returned Zero")
	}
}

func TestStringParseRoundTrip(t *testing.T) {
	for i := 0; i < 100; i++ {
		x := New()
		got, err := Parse(x.String())
		if err != nil {
			t.Fatalf("Parse(%q): %v", x.String(), err)
		}
		if got != x {
			t.Fatalf("round-trip mismatch: %v != %v", got, x)
		}
	}
}

func TestParseRejectsBadLength(t *testing.T) {
	for _, s := range []string{"", "abcd", strings.Repeat("a", 31), strings.Repeat("a", 33), strings.Repeat("g", 32)} {
		if _, err := Parse(s); err != ErrBadLength {
			t.Fatalf("Parse(%q): want ErrBadLength, got %v", s, err)
		}
	}
}

func TestParseAcceptsUppercase(t *testing.T) {
	x := New()
	upper := strings.ToUpper(x.String())
	got, err := Parse(upper)
	if err != nil {
		t.Fatalf("Parse(uppercase): %v", err)
	}
	if got != x {
		t.Fatalf("uppercase parse mismatch")
	}
	if got.String() != strings.ToLower(upper) {
		t.Fatalf("String did not re-emit lowercase")
	}
}

func TestXORIdentityAndInvolution(t *testing.T) {
	if XOR() != Zero {
		t.Fatal("XOR() != Zero")
	}
	x := New()
	if XOR(x) != x {
		t.Fatal("XOR(x) != x")
	}
	if XOR(x, x) != Zero {
		t.Fatal("XOR(x,x) != Zero")
	}
}

func TestXORCommutativeAssociative(t *testing.T) {
	a, b, c := New(), New(), New()
	if XOR(a, b, c) != XOR(c, a, b) {
		t.Fatal("XOR not order-independent")
	}
	if XOR(a, b, c) != XOR(XOR(a, b), c) {
		t.Fatal("XOR not associative")
	}
}

func TestJSONTextMarshal(t *testing.T) {
	type rec struct {
		ID    ID         `json:"id"`
		Names map[ID]int `json:"names"`
	}
	x := New()
	in := rec{ID: x, Names: map[ID]int{x: 7}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), x.String()) {
		t.Fatalf("hex not in JSON: %s", b)
	}
	var out rec
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ID != x || out.Names[x] != 7 {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}
