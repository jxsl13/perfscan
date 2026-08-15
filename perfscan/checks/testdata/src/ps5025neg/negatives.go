package ps5025neg

import (
	"bytes"
	"strings"
)

// None of these may be reported.
func negatives(s, cut string, b []byte) {
	_ = strings.LastIndexAny(s, "ab")      // two characters: a genuine SET search LastIndexByte cannot express
	_ = bytes.LastIndexAny(b, "=;")        // two characters
	_ = bytes.LastIndexAny(b, "é")         // ONE rune but TWO bytes of UTF-8: the length rule is bytes, not runes
	_ = strings.LastIndexAny(s, "…")       // three bytes
	_ = strings.LastIndexAny(s, "")        // empty cutset: LastIndexAny is a constant -1
	_ = bytes.LastIndexAny(b, "")          // empty cutset
	_ = strings.LastIndexAny(s, cut)       // not a constant: length and content unknown
	_ = strings.LastIndexAny(s, s[:1])     // one byte long at runtime, but not a constant
	_ = bytes.LastIndexAny(b, cut+"z")     // arbitrary non-constant cutset expression
	_ = strings.LastIndex(s, "z")          // the substring forms are PS5007/PS5013's territory
	_ = bytes.LastIndex(b, []byte("z"))    // ditto
	_ = strings.IndexAny(s, "z")           // the forward scan is PS5022's territory
	_ = bytes.IndexAny(b, "z")             // ditto
	_ = strings.ContainsAny(s, "z")        // ditto
	_ = strings.LastIndexByte(s, 'z')      // already the byte form
	_ = bytes.LastIndexByte(b, 'z')        // already the byte form
	_ = strings.LastIndexFunc(s, func(rune) bool { return false }) // a predicate, not a cutset
}

// THE load-bearing exclusion: a single byte >= 0x80 makes LastIndexAny
// remap the cutset rune to utf8.RuneError and search for the LAST
// INVALID-UTF-8 position — which is NOT LastIndexByte(s, 0xff). Never
// matched, not even advisory: no byte-search rewrite exists for these.
func nonASCII(s string, b []byte) {
	_ = strings.LastIndexAny(s, "\xff")
	_ = bytes.LastIndexAny(b, "\xff")
	_ = strings.LastIndexAny(s, "\x80")
	_ = bytes.LastIndexAny(b, "\x80")
}

// A same-named method on a shadowing identifier is not the stdlib
// function: type information rejects it (its package is not
// "strings"/"bytes").
type fake struct{}

func (fake) LastIndexAny(s, chars string) int { return -1 }

func shadowed(s string) int {
	strings := fake{}
	return strings.LastIndexAny(s, "z")
}

// A func value loses the callee shape: the call's Fun is a plain
// identifier, never a strings/bytes selector.
func funcValue(s string) int {
	f := strings.LastIndexAny
	return f(s, "z")
}
