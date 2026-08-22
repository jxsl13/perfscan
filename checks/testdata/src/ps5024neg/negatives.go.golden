package ps5024neg

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// None of these may be reported.
func negatives(s string, b []byte, r rune) {
	_ = strings.ContainsRune(s, r)                                // not a constant: the rune-class branch is decided at runtime
	_ = strings.ContainsRune(s, r+1)                              // non-constant expression
	_ = strings.ContainsRune(s, rune(s[0]))                       // runtime value
	_ = strings.Contains(s, "z")                                  // the string-needle forms are PS5016/PS5014's territory
	_ = strings.ContainsAny(s, "z")                               // the cutset form is PS5022's territory
	_ = strings.IndexRune(s, 'z')                                 // the index form is a separate pattern, out of scope
	_ = bytes.ContainsRune(b, 'z')                                // the bytes twin is a separate pattern, out of scope
	_ = strings.IndexByte(s, 'z')                                 // already the byte form
	_ = strings.ContainsFunc(s, func(rune) bool { return false }) // predicate, not a rune
}

// THE load-bearing exclusion: a constant rune outside [0, 0x80) takes a
// DIFFERENT IndexRune branch — a multi-byte rune is a substring search
// over its UTF-8 encoding, utf8.RuneError searches for the first
// invalid-UTF-8 position, and an invalid or negative rune is a constant
// false — none of which IndexByte with a truncated byte can express.
// Never matched, not even advisory: no byte-search rewrite exists.
func nonASCII(s string) {
	_ = strings.ContainsRune(s, 'é')            // U+00E9: two bytes of UTF-8
	_ = strings.ContainsRune(s, '\u00e9')       // same rune, escape spelling
	_ = strings.ContainsRune(s, '€')            // three bytes
	_ = strings.ContainsRune(s, '你')            // three bytes
	_ = strings.ContainsRune(s, 0x80)           // first non-ASCII value: already two bytes of UTF-8
	_ = strings.ContainsRune(s, '\ufffd')       // utf8.RuneError: searches for invalid UTF-8, not the byte
	_ = strings.ContainsRune(s, utf8.RuneError) // same, via the named constant
	_ = strings.ContainsRune(s, 0xD800)         // surrogate: constant false
	_ = strings.ContainsRune(s, 0x110000)       // > MaxRune: constant false
	_ = strings.ContainsRune(s, -1)             // negative: constant false
}

// A same-named method on a shadowing identifier is not the stdlib
// function: type information rejects it (its package is not "strings").
type fake struct{}

func (fake) ContainsRune(s string, r rune) bool { return false }

func shadowed(s string) bool {
	strings := fake{}
	return strings.ContainsRune(s, 'z')
}

// A func value loses the callee shape: the call's Fun is a plain
// identifier, never a strings selector.
func funcValue(s string) bool {
	f := strings.ContainsRune
	return f(s, 'z')
}
