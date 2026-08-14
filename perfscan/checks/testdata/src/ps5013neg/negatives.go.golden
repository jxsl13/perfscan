package ps5013neg

import (
	"bytes"
	"strings"
)

// None of these may be reported.
func negatives(b, sep []byte, s string, c byte) {
	_ = bytes.Index(b, []byte{'a', 'b'})   // two elements: the byte scan cannot express it
	_ = bytes.Index(b, []byte("ab"))       // two bytes, same story
	_ = bytes.Index(b, []byte("é"))        // ONE rune but TWO bytes of UTF-8: the length rule is bytes, not runes
	_ = bytes.LastIndex(b, []byte("…"))    // three bytes
	_ = bytes.Index(b, []byte{})           // empty needle: Index(b, empty) == 0, inexpressible with IndexByte
	_ = bytes.LastIndex(b, []byte(""))     // LastIndex(b, empty) == len(b), same story
	_ = bytes.Index(b, nil)                // nil needle behaves as empty
	_ = bytes.Index(b, sep)                // not statically one byte
	_ = bytes.LastIndex(b, []byte(s))      // conversion of a NON-constant string: length unknown
	_ = bytes.Index(b, append(sep, c))     // arbitrary needle expression
	_ = bytes.Index(b, []byte{0: c})       // keyed one-element literal: a deliberate index spelling, out of scope
	_ = bytes.IndexByte(b, c)              // already the byte form
	_ = bytes.LastIndexByte(b, c)          // already the byte form
	_ = bytes.IndexAny(b, "z")             // a SET needle — a different pattern with different multi-byte semantics
	_ = bytes.LastIndexAny(b, "z")         // same
	_ = bytes.IndexRune(b, 'z')            // rune search, not substring search
	_ = bytes.Count(b, []byte{'z'})        // other bytes members are out of scope
	_ = bytes.Contains(b, []byte{'z'})     // same
	_ = strings.Index(s, "z")              // the strings twin is PS5007's territory
	_ = strings.LastIndex(s, "z")          // same
}

// A same-named method on a shadowing identifier is not the stdlib
// function: type information rejects it (its package is not "bytes").
type fake struct{}

func (fake) Index(b, sep []byte) int     { return 0 }
func (fake) LastIndex(b, sep []byte) int { return 0 }

func shadowed(b []byte) int {
	bytes := fake{}
	return bytes.Index(b, []byte{'z'}) + bytes.LastIndex(b, []byte{'z'})
}

// A func value loses the callee shape: the call's Fun is a plain
// identifier, never a bytes selector.
func funcValue(b []byte) int {
	f := bytes.Index
	return f(b, []byte{'z'})
}
