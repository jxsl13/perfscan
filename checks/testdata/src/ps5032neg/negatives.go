package ps5032neg

import (
	"bytes"
	"strings"
)

// None of these may be reported.
func negatives(s string, cut string, b []byte) {
	_ = bytes.IndexAny(b, "z")        // one ASCII byte: IndexAny's own one-byte fast path (and the ASCII sibling's territory)
	_ = bytes.ContainsAny(b, "/")     // ditto
	_ = bytes.IndexAny(b, "")         // empty cutset: IndexAny is a constant -1 / ContainsAny false
	_ = bytes.IndexAny(b, "—…")       // two runes: a genuine SET search IndexRune cannot express
	_ = bytes.ContainsAny(b, "ab")    // two ASCII characters
	_ = bytes.IndexAny(b, "—x")       // one multi-byte rune plus trailing garbage: still a set
	_ = bytes.ContainsAny(b, "x—")    // ditto, other order
	_ = bytes.IndexAny(b, cut)        // not a constant: length and content unknown
	_ = bytes.ContainsAny(b, s[:2])   // two bytes long at runtime, but not a constant
	_ = strings.IndexAny(s, "—")      // the strings twins are PS5030's territory, out of scope here
	_ = strings.ContainsAny(s, "€")   // ditto
	_ = bytes.LastIndexAny(b, "—")    // the backward scan is a separate pattern, out of scope
	_ = bytes.IndexRune(b, '—')       // already the rune form
	_ = bytes.ContainsRune(b, '—')    // already the rune form
	_ = bytes.Index(b, []byte("—"))   // the substring forms are other checks' territory
	_ = bytes.Contains(b, []byte("—")) // ditto
}

// Load-bearing exclusions: cutsets whose bytes do NOT decode as exactly
// one valid non-RuneError rune. A lone non-ASCII byte, a truncated or
// overlong sequence and a surrogate encoding all decode as
// RuneError-per-byte (per-byte membership semantics in IndexAny's
// general loop — no single-rune equivalent), and the real U+FFFD cutset
// is excluded conservatively because IndexRune's documented RuneError
// contract answers a different question (the first INVALID sequence).
// Never matched, not even advisory.
func excludedCutsets(b []byte) {
	_ = bytes.IndexAny(b, "\xff")         // lone non-ASCII byte
	_ = bytes.ContainsAny(b, "\x80")      // lone continuation byte
	_ = bytes.IndexAny(b, "\xe2\x80")     // truncated three-byte encoding
	_ = bytes.ContainsAny(b, "\xc0\xaf")  // overlong encoding of '/'
	_ = bytes.IndexAny(b, "\xed\xa0\x80") // surrogate half
	_ = bytes.ContainsAny(b, "\uFFFD")   // the real U+FFFD, escape spelling
	_ = bytes.IndexAny(b, "\xef\xbf\xbd") // the real U+FFFD, byte spelling
}

// A same-named method on a shadowing identifier is not the stdlib
// function: type information rejects it (its package is not "bytes").
type fake struct{}

func (fake) IndexAny(b []byte, chars string) int     { return -1 }
func (fake) ContainsAny(b []byte, chars string) bool { return false }

func shadowed(b []byte) bool {
	bytes := fake{}
	return bytes.IndexAny(b, "—") >= 0 || bytes.ContainsAny(b, "—")
}

// A func value loses the callee shape: the call's Fun is a plain
// identifier, never a bytes selector.
func funcValue(b []byte) int {
	f := bytes.IndexAny
	return f(b, "—")
}
