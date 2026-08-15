package ps5014neg

import (
	"bytes"
	"strings"
)

// None of these may be reported.
func negatives(b, sep []byte, s string, c byte) {
	_ = bytes.Contains(b, []byte{'a', 'b'}) // two elements: the byte scan cannot express it
	_ = bytes.Contains(b, []byte("ab"))     // two bytes, same story
	_ = bytes.Contains(b, []byte("é"))      // ONE rune but TWO bytes of UTF-8: the length rule is bytes, not runes
	_ = bytes.Contains(b, []byte("…"))      // three bytes
	_ = bytes.Contains(b, []byte{})         // empty needle: Contains(b, empty) is always true, inexpressible with IndexByte
	_ = bytes.Contains(b, []byte(""))       // same story
	_ = bytes.Contains(b, nil)              // nil needle behaves as empty
	_ = bytes.Contains(b, sep)              // not statically one byte
	_ = bytes.Contains(b, []byte(s))        // conversion of a NON-constant string: length unknown
	_ = bytes.Contains(b, append(sep, c))   // arbitrary needle expression
	_ = bytes.Contains(b, []byte{0: c})     // keyed one-element literal: a deliberate index spelling, out of scope
	_ = bytes.ContainsAny(b, "z")           // a SET needle — a different pattern with different multi-byte semantics
	_ = bytes.ContainsRune(b, 'z')          // rune membership, not substring search
	_ = bytes.ContainsFunc(b, func(r rune) bool { return r == 'z' }) // predicate membership
	_ = bytes.Index(b, []byte{'z'})         // the index form is PS5013's territory
	_ = bytes.IndexByte(b, c) >= 0          // already the byte form
	_ = strings.Contains(s, "z")            // the strings twin is out of scope
}

// A same-named method on a shadowing identifier is not the stdlib
// function: type information rejects it (its package is not "bytes").
type fake struct{}

func (fake) Contains(b, sep []byte) bool { return false }

func shadowed(b []byte) bool {
	bytes := fake{}
	return bytes.Contains(b, []byte{'z'})
}

// A func value loses the callee shape: the call's Fun is a plain
// identifier, never a bytes selector.
func funcValue(b []byte) bool {
	f := bytes.Contains
	return f(b, []byte{'z'})
}
