package ps5026neg

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// None of these may be reported.
func negatives(s string, b []byte, r rune) {
	_ = bytes.ContainsRune(b, r)                                // not a constant: the rune-class branch is decided at runtime
	_ = bytes.ContainsRune(b, r+1)                              // non-constant expression
	_ = bytes.ContainsRune(b, rune(b[0]))                       // runtime value
	_ = bytes.Contains(b, []byte("z"))                          // the byte-slice-needle form is a separate pattern
	_ = bytes.ContainsAny(b, "z")                               // the cutset form is a separate pattern
	_ = bytes.IndexRune(b, 'z')                                 // the index form is PS5023's territory
	_ = strings.ContainsRune(s, 'z')                            // the strings twin is PS5024's territory
	_ = bytes.IndexByte(b, 'z')                                 // already the byte form
	_ = bytes.ContainsFunc(b, func(rune) bool { return false }) // predicate, not a rune
}

// THE load-bearing exclusion: a constant rune outside [0, 0x80) takes a
// DIFFERENT IndexRune branch — a multi-byte rune is a substring search
// over its UTF-8 encoding, utf8.RuneError searches for the first
// invalid-UTF-8 position, and an invalid or negative rune is a constant
// false — none of which IndexByte with a truncated byte can express.
// Never matched, not even advisory: no byte-search rewrite exists.
func nonASCII(b []byte) {
	_ = bytes.ContainsRune(b, 'é')            // U+00E9: two bytes of UTF-8
	_ = bytes.ContainsRune(b, '\u00e9')       // same rune, escape spelling
	_ = bytes.ContainsRune(b, '€')            // three bytes
	_ = bytes.ContainsRune(b, '你')            // three bytes
	_ = bytes.ContainsRune(b, 0x80)           // first non-ASCII value: already two bytes of UTF-8
	_ = bytes.ContainsRune(b, '\ufffd')       // utf8.RuneError: searches for invalid UTF-8, not the byte
	_ = bytes.ContainsRune(b, utf8.RuneError) // same, via the named constant
	_ = bytes.ContainsRune(b, 0xD800)         // surrogate: constant false
	_ = bytes.ContainsRune(b, 0x110000)       // > MaxRune: constant false
	_ = bytes.ContainsRune(b, -1)             // negative: constant false
}

// A same-named method on a shadowing identifier is not the stdlib
// function: type information rejects it (its package is not "bytes").
type fake struct{}

func (fake) ContainsRune(b []byte, r rune) bool { return false }

func shadowed(b []byte) bool {
	bytes := fake{}
	return bytes.ContainsRune(b, 'z')
}

// A func value loses the callee shape: the call's Fun is a plain
// identifier, never a bytes selector.
func funcValue(b []byte) bool {
	f := bytes.ContainsRune
	return f(b, 'z')
}
