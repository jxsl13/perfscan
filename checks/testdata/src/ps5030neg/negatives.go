package ps5030neg

import (
	"bytes"
	"strings"
)

// None of these may be reported.
func negatives(s, cut string, b []byte) {
	_ = strings.IndexAny(s, "z")         // one ASCII byte: PS5022's territory
	_ = strings.ContainsAny(s, "/")      // ditto
	_ = strings.IndexAny(s, "")          // empty cutset: IndexAny is a constant -1 / ContainsAny false
	_ = strings.IndexAny(s, "—…")        // two runes: a genuine SET search IndexRune cannot express
	_ = strings.ContainsAny(s, "ab")     // two ASCII characters
	_ = strings.IndexAny(s, "—x")        // one multi-byte rune plus trailing garbage: still a set
	_ = strings.ContainsAny(s, "x—")     // ditto, other order
	_ = strings.IndexAny(s, cut)         // not a constant: length and content unknown
	_ = strings.ContainsAny(s, s[:2])    // two bytes long at runtime, but not a constant
	_ = bytes.IndexAny(b, "—")           // the bytes twins are a different implementation, out of scope
	_ = bytes.ContainsAny(b, "€")        // ditto
	_ = strings.LastIndexAny(s, "—")     // the backward scan is a separate pattern, out of scope
	_ = strings.IndexRune(s, '—')        // already the rune form
	_ = strings.ContainsRune(s, '—')     // already the rune form
	_ = strings.Index(s, "—")            // the substring forms are other checks' territory
	_ = strings.Contains(s, "—")         // ditto
}

// Load-bearing exclusions: cutsets whose bytes do NOT decode as exactly
// one valid non-RuneError rune. A lone non-ASCII byte, a truncated or
// overlong sequence and a surrogate encoding all decode as
// RuneError-per-byte (per-byte membership semantics in IndexAny's
// fallback — no single-rune equivalent), and the real U+FFFD cutset is
// excluded conservatively because IndexRune's documented RuneError
// contract answers a different question (the first INVALID sequence).
// Never matched, not even advisory.
func excludedCutsets(s string) {
	_ = strings.IndexAny(s, "\xff")         // lone non-ASCII byte
	_ = strings.ContainsAny(s, "\x80")      // lone continuation byte
	_ = strings.IndexAny(s, "\xe2\x80")     // truncated three-byte encoding
	_ = strings.ContainsAny(s, "\xc0\xaf")  // overlong encoding of '/'
	_ = strings.IndexAny(s, "\xed\xa0\x80") // surrogate half
	_ = strings.ContainsAny(s, "\uFFFD")   // the real U+FFFD, escape spelling
	_ = strings.IndexAny(s, "\xef\xbf\xbd") // the real U+FFFD, byte spelling
}

// A same-named method on a shadowing identifier is not the stdlib
// function: type information rejects it (its package is not "strings").
type fake struct{}

func (fake) IndexAny(s, chars string) int     { return -1 }
func (fake) ContainsAny(s, chars string) bool { return false }

func shadowed(s string) bool {
	strings := fake{}
	return strings.IndexAny(s, "—") >= 0 || strings.ContainsAny(s, "—")
}

// A func value loses the callee shape: the call's Fun is a plain
// identifier, never a strings selector.
func funcValue(s string) int {
	f := strings.IndexAny
	return f(s, "—")
}
