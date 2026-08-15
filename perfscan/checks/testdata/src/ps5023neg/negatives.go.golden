package ps5023neg

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// None of these may be reported.

// THE load-bearing exclusions. A non-constant rune never matches, not
// even advisory: a variable can hold a non-ASCII value at runtime, where
// IndexRune searches for the rune's multi-byte UTF-8 encoding while
// IndexByte(s, byte(r)) searches for a single TRUNCATED byte — genuinely
// different answers. A constant >= 0x80 is a multi-byte UTF-8 search
// ('é' looks for C3 A9, never the single byte E9), utf8.RuneError
// (0xFFFD) returns the first INVALID-UTF-8 position instead of searching,
// and a negative constant is IndexRune's constant -1 via !utf8.ValidRune
// — none of which IndexByte can express, so no rewrite exists for any of
// them.
func excluded(s string, b []byte, r rune) {
	_ = strings.IndexRune(s, r)              // not a constant: byte(r) would truncate a non-ASCII value
	_ = bytes.IndexRune(b, r)                // ditto
	_ = strings.IndexRune(s, rune(s[0]))     // not a constant either
	_ = strings.IndexRune(s, 'é')            // 0xE9 >= 0x80: two-byte UTF-8 search
	_ = strings.IndexRune(s, '\u00e9')     // same rune, escape spelling
	_ = strings.IndexRune(s, '\x80')         // the first non-ASCII value: the bound is exclusive
	_ = bytes.IndexRune(b, 'あ')             // three-byte UTF-8 search
	_ = strings.IndexRune(s, utf8.RuneError) // invalid-UTF-8 probe, not a byte search
	_ = strings.IndexRune(s, -1)             // negative: IndexRune is a constant -1 here
	_ = bytes.IndexRune(b, 0x10FFFF+1)       // beyond the rune range: also the constant -1 path
}

// The sibling shapes belong to other checks (or to no check at all).
func siblings(s string, b []byte) {
	_ = strings.ContainsRune(s, 'z') // splices in a comparison — a separate pattern, out of scope
	_ = bytes.ContainsRune(b, 'z')   // ditto
	_ = strings.IndexByte(s, 'z')    // already the byte form
	_ = bytes.IndexByte(b, 'z')      // already the byte form
	_ = strings.Index(s, "z")        // one-byte substring needles are PS5007/PS5013's territory
	_ = bytes.Contains(b, []byte("z"))
	_ = strings.IndexAny(s, "z") // one-ASCII-byte cutsets are PS5022's territory
}

// A same-named method on a shadowing identifier is not the stdlib
// function: type information rejects it (its package is not
// "strings"/"bytes").
type fake struct{}

func (fake) IndexRune(s string, r rune) int { return -1 }

func shadowed(s string) int {
	strings := fake{}
	return strings.IndexRune(s, 'z')
}

// A func value loses the callee shape: the call's Fun is a plain
// identifier, never a strings/bytes selector.
func funcValue(s string) int {
	f := strings.IndexRune
	return f(s, 'z')
}
