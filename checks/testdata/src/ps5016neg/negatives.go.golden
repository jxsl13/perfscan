package ps5016neg

import (
	"bytes"
	"strings"
)

// None of these may be reported.
func negatives(s, sub string, b []byte) {
	_ = strings.Contains(s, "ab")      // two bytes: the byte scan cannot express it
	_ = strings.Contains(s, "é")       // ONE rune but TWO bytes of UTF-8: the length rule is bytes, not runes
	_ = strings.Contains(s, "…")       // three bytes
	_ = strings.Contains(s, "")        // empty needle: Contains(s, "") is always true, inexpressible with IndexByte
	_ = strings.Contains(s, sub)       // not a constant: length unknown
	_ = strings.Contains(s, s[:1])     // one byte long at runtime, but not a constant
	_ = strings.Contains(s, sub+"z")   // arbitrary non-constant needle expression
	_ = strings.ContainsAny(s, "z")    // a SET needle — a different pattern with different multi-byte semantics
	_ = strings.ContainsRune(s, 'z')   // rune membership, not substring search
	_ = strings.ContainsFunc(s, func(r rune) bool { return r == 'z' }) // predicate membership
	_ = strings.Index(s, "z")          // the index form is PS5007's territory
	_ = strings.IndexByte(s, 'z') >= 0 // already the byte form
	_ = bytes.Contains(b, []byte("z")) // the bytes twin is PS5014's territory
}

// A same-named method on a shadowing identifier is not the stdlib
// function: type information rejects it (its package is not "strings").
type fake struct{}

func (fake) Contains(s, sub string) bool { return false }

func shadowed(s string) bool {
	strings := fake{}
	return strings.Contains(s, "z")
}

// A func value loses the callee shape: the call's Fun is a plain
// identifier, never a strings selector.
func funcValue(s string) bool {
	f := strings.Contains
	return f(s, "z")
}
