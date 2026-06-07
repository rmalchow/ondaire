// Package id defines the node/group identifier: 16 random bytes rendered as
// 32 lowercase hex chars. A group ID is the bytewise XOR of its member IDs
// (§5). ID implements encoding.TextMarshaler/TextUnmarshaler so it works as a
// hex string in JSON struct fields and as a map[ID]… key.
package id

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
)

// ID is a node or group identifier: 16 bytes, rendered as 32 lowercase hex chars.
type ID [16]byte

// Zero is the all-zero ID (XOR identity; also the "no group" sentinel).
var Zero ID

// ErrBadLength is returned by Parse for inputs that are not 32 hex chars.
var ErrBadLength = errors.New("id: want 32 hex chars")

// New returns a cryptographically-random ID. Panics only if crypto/rand fails.
func New() ID {
	var i ID
	if _, err := rand.Read(i[:]); err != nil {
		panic("id: crypto/rand failed: " + err.Error())
	}
	return i
}

// Parse decodes exactly 32 lowercase-or-uppercase hex chars into an ID.
func Parse(s string) (ID, error) {
	var i ID
	if len(s) != 32 {
		return Zero, ErrBadLength
	}
	if _, err := hex.Decode(i[:], []byte(s)); err != nil {
		return Zero, ErrBadLength
	}
	return i, nil
}

// MustParse panics on error; for tests and constants.
func MustParse(s string) ID {
	i, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return i
}

// String renders the ID as 32 lowercase hex chars.
func (i ID) String() string {
	return hex.EncodeToString(i[:])
}

// IsZero reports whether i == Zero.
func (i ID) IsZero() bool {
	return i == Zero
}

// XOR returns the bytewise XOR of all arguments (commutative, associative).
// XOR() == Zero; XOR(x) == x; XOR(x,x) == Zero. This is the group-ID rule (§5).
func XOR(ids ...ID) ID {
	var out ID
	for _, x := range ids {
		for b := range out {
			out[b] ^= x[b]
		}
	}
	return out
}

// MarshalText renders the ID as lowercase hex (JSON value and map key).
func (i ID) MarshalText() ([]byte, error) {
	dst := make([]byte, hex.EncodedLen(len(i)))
	hex.Encode(dst, i[:])
	return dst, nil
}

// UnmarshalText parses 32 hex chars into the ID.
func (i *ID) UnmarshalText(b []byte) error {
	v, err := Parse(string(b))
	if err != nil {
		return err
	}
	*i = v
	return nil
}
