package ps5022neg

import (
	"bytes"
	"strings"
)

// None of these may be reported.
func negatives(s, cut string, b []byte) {
	_ = strings.IndexAny(s, "ab")      // two characters: a genuine SET search IndexByte cannot express
	_ = strings.ContainsAny(s, "=;")   // two characters
	_ = bytes.IndexAny(b, "é")         // ONE rune but TWO bytes of UTF-8: the length rule is bytes, not runes
	_ = strings.ContainsAny(s, "…")    // three bytes
	_ = strings.IndexAny(s, "")        // empty cutset: IndexAny is a constant -1 / ContainsAny false
	_ = bytes.ContainsAny(b, "")       // empty cutset
	_ = strings.IndexAny(s, cut)       // not a constant: length and content unknown
	_ = strings.ContainsAny(s, s[:1])  // one byte long at runtime, but not a constant
	_ = bytes.IndexAny(b, cut+"z")     // arbitrary non-constant cutset expression
	_ = strings.Index(s, "z")          // the substring forms are PS5007/PS5013/PS5016/PS5014's territory
	_ = strings.Contains(s, "z")       // ditto
	_ = bytes.Contains(b, []byte("z")) // ditto
	_ = strings.LastIndexAny(s, "z")   // the backward scan is a separate pattern, out of scope
	_ = bytes.LastIndexAny(b, "z")     // ditto
	_ = strings.ContainsRune(s, 'z')   // rune membership, not a cutset
	_ = strings.IndexByte(s, 'z')      // already the byte form
	_ = bytes.IndexByte(b, 'z')        // already the byte form
}

// THE load-bearing exclusion: a single byte >= 0x80 makes IndexAny remap
// the cutset rune to utf8.RuneError and search for the first INVALID-UTF-8
// position — which is NOT IndexByte(s, 0xff). Never matched, not even
// advisory: no byte-search rewrite exists for these.
func nonASCII(s string, b []byte) {
	_ = strings.IndexAny(s, "\xff")
	_ = strings.ContainsAny(s, "\xff")
	_ = bytes.IndexAny(b, "\xff")
	_ = bytes.ContainsAny(b, "\x80")
	_ = strings.IndexAny(s, "\x80")
}

// A same-named method on a shadowing identifier is not the stdlib
// function: type information rejects it (its package is not
// "strings"/"bytes").
type fake struct{}

func (fake) IndexAny(s, chars string) int     { return -1 }
func (fake) ContainsAny(s, chars string) bool { return false }

func shadowed(s string) bool {
	strings := fake{}
	return strings.IndexAny(s, "z") >= 0 || strings.ContainsAny(s, "z")
}

// A func value loses the callee shape: the call's Fun is a plain
// identifier, never a strings/bytes selector.
func funcValue(s string) int {
	f := strings.IndexAny
	return f(s, "z")
}
