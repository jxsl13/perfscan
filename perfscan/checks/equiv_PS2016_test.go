package checks

// Runtime differential for PS2016: string(bytes.TrimFunc([]byte(s), f))
// vs strings.TrimFunc(s, f), plus the TrimLeftFunc/TrimRightFunc
// siblings. The fix's safety argument is that both packages share one
// rune-iteration core — TrimFunc is TrimRightFunc(TrimLeftFunc(s, f), f)
// in both, TrimLeftFunc scans forward with indexFunc and TrimRightFunc
// scans backward with lastIndexFunc, and the two packages' decoders
// (utf8.DecodeRune vs DecodeRuneInString, DecodeLastRune vs
// DecodeLastRuneInString) agree on every byte sequence, yielding
// RuneError with width 1 on any invalid one. So f is called on the SAME
// runes in the SAME order and count in both forms, the trim boundaries
// coincide, and the resulting string VALUE is identical for every
// (s, f) pair. This suite pins BOTH claims — the value identity and the
// exact f call sequence (order and count, hence side effects) — over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 4 over an
//     adversarial alphabet (ASCII members and non-members, NUL, the
//     bytes of multi-byte runes so truncated and complete sequences
//     both arise, a bare continuation byte, an invalid lead byte, and
//     0xFF), crossed with predicates exercising every decision path:
//     always/never, ASCII-only, non-ASCII-only, matching U+FFFD (which
//     must fire on every INVALID sequence, not the replacement char
//     alone), and the common unicode.Is* table predicates;
//   - targeted seeds (fully-trimmed inputs where bytes historically
//     returns nil, boundary-straddling multi-byte runes, invalid UTF-8
//     on both sides, very long inputs);
//   - randomized long inputs over the full byte range with a fixed
//     seed.
//
// It also pins the perf premise: the strings twins perform zero
// allocations (they return a substring), which is the entire win.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unicode"
)

// ps2016Before* are the exact Before-shapes of the check.
func ps2016BeforeTrim(s string, f func(rune) bool) string {
	return string(bytes.TrimFunc([]byte(s), f))
}
func ps2016BeforeLeft(s string, f func(rune) bool) string {
	return string(bytes.TrimLeftFunc([]byte(s), f))
}
func ps2016BeforeRight(s string, f func(rune) bool) string {
	return string(bytes.TrimRightFunc([]byte(s), f))
}

// ps2016After* are the exact After-shapes of the check.
func ps2016AfterTrim(s string, f func(rune) bool) string  { return strings.TrimFunc(s, f) }
func ps2016AfterLeft(s string, f func(rune) bool) string  { return strings.TrimLeftFunc(s, f) }
func ps2016AfterRight(s string, f func(rune) bool) string { return strings.TrimRightFunc(s, f) }

// ps2016Check verifies, for one (s, f) pair and all three functions,
// that the Before and After forms return the identical string AND call
// f on the identical rune sequence — order and count — so any side
// effects inside f are preserved exactly by the rewrite.
func ps2016Check(t *testing.T, s string, f func(rune) bool, fname string) {
	t.Helper()
	type form struct {
		name   string
		before func(string, func(rune) bool) string
		after  func(string, func(rune) bool) string
	}
	forms := []form{
		{"TrimFunc", ps2016BeforeTrim, ps2016AfterTrim},
		{"TrimLeftFunc", ps2016BeforeLeft, ps2016AfterLeft},
		{"TrimRightFunc", ps2016BeforeRight, ps2016AfterRight},
	}
	for _, fo := range forms {
		var beforeCalls, afterCalls []rune
		before := fo.before(s, func(r rune) bool { beforeCalls = append(beforeCalls, r); return f(r) })
		after := fo.after(s, func(r rune) bool { afterCalls = append(afterCalls, r); return f(r) })
		if before != after {
			t.Fatalf("%s/%s value divergence on %q:\n before=%q\n after=%q", fo.name, fname, s, before, after)
		}
		if len(beforeCalls) != len(afterCalls) {
			t.Fatalf("%s/%s predicate call-count divergence on %q: before saw %v, after saw %v", fo.name, fname, s, beforeCalls, afterCalls)
		}
		for i := range beforeCalls {
			if beforeCalls[i] != afterCalls[i] {
				t.Fatalf("%s/%s predicate call-sequence divergence on %q at call %d: before saw %v, after saw %v", fo.name, fname, s, i, beforeCalls, afterCalls)
			}
		}
	}
}

// ps2016Preds exercises every decision path of the shared trim core.
var ps2016Preds = []struct {
	name string
	f    func(rune) bool
}{
	{"never", func(rune) bool { return false }},                      // nothing trimmed; bytes.TrimLeftFunc keeps s whole
	{"always", func(rune) bool { return true }},                      // fully trimmed: bytes historically returns nil, string(nil) == ""
	{"IsSpace", unicode.IsSpace},                                     // the canonical table predicate
	{"asciiOnly", func(r rune) bool { return r < 0x80 && r != 'x' }}, // ASCII decisions; multi-byte and invalid runes never match
	{"nonASCII", func(r rune) bool { return r >= 0x80 }},             // matches every multi-byte rune AND RuneError (invalid bytes)
	{"isFFFD", func(r rune) bool { return r == 0xFFFD }},             // must fire on every INVALID sequence, not just a literal U+FFFD
	{"threeByte", func(r rune) bool { return r == '\u3042' }},        // one specific three-byte rune
	{"IsLetter", unicode.IsLetter},                                   // wide table predicate across the plane
}

func TestEquiv_PS2016_ExhaustiveShort(t *testing.T) {
	alphabet := []byte{
		' ', '\t', // ASCII trimmables
		'a', 'x', // ASCII letters (x is asciiOnly's carve-out)
		0x00,       // NUL
		0xC3, 0xA9, // C3 A9 = U+00E9; each byte alone is invalid UTF-8
		0xE3, 0x81, 0x82, // E3 81 82 = U+3042; prefixes are truncated sequences
		0xA0, // bare continuation byte
		0xFF, // never valid in UTF-8
	}
	var buf [4]byte
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			s := string(buf[:n])
			for _, p := range ps2016Preds {
				ps2016Check(t, s, p.f, p.name)
			}
			return
		}
		for _, c := range alphabet {
			buf[depth] = c
			rec(n, depth+1)
		}
	}
	for n := 0; n <= len(buf); n++ {
		rec(n, 0)
	}
}

func TestEquiv_PS2016_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("payload/", 4096)
	seeds := []string{
		"",
		" ",
		"    ", // fully trimmed by IsSpace: bytes historically returns nil, string(nil) == ""
		"  x  ",
		"ééxé",                               // multi-byte non-space edges
		"\u3042日本\u3042",                     // three-byte members wrapping three-byte non-members
		"\xc3\x28",                           // invalid two-byte sequence
		" \xc3\x28 ",                         // invalid UTF-8 inside the trimmed span
		"\x80\x80",                           // bare continuation bytes only
		" \xff\xfe\x80 ",                     // invalid bytes wrapped in spaces
		"\xc3",                               // truncated U+00E9 prefix at the very edge
		"\xc3 ",                              // truncated sequence then a space
		" \xc3",                              // space then truncated sequence
		"\xe3\x81",                           // truncated U+3042 prefix
		"a\x00b",                             // NUL inside the kept span
		"�x�",                                // literal replacement chars at the edges
		"\u0085\u00a0x\u00a0\u0085",          // multi-byte Unicode spaces at both edges
		"  " + long + "  ",                   // long payload, trimmed ends
		strings.Repeat(" ", 8192) + "x",      // long all-trimmed prefix
		"x" + strings.Repeat("\u3042", 2048), // long multi-byte suffix
		strings.Repeat(" é", 4096),           // long input walked rune by rune
	}
	for _, s := range seeds {
		for _, p := range ps2016Preds {
			ps2016Check(t, s, p.f, p.name)
		}
	}
}

func TestEquiv_PS2016_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25016))
	randBytes := func(n int, pick func() byte) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = pick()
		}
		return string(b)
	}
	// Full byte range: mostly invalid UTF-8 at random alignments, so the
	// forward and backward decoders are exercised on every malformed
	// shape.
	anyByte := func() byte { return byte(rng.Intn(256)) }
	for range 50000 {
		s := randBytes(rng.Intn(33), anyByte)
		for _, p := range ps2016Preds {
			ps2016Check(t, s, p.f, p.name)
		}
	}
	// Trimmable-heavy alphabet so trims routinely straddle rune
	// boundaries of multi-byte runes.
	heavy := []byte{' ', '\t', 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF, 'x'}
	heavyByte := func() byte { return heavy[rng.Intn(len(heavy))] }
	for range 50000 {
		s := randBytes(rng.Intn(25), heavyByte)
		for _, p := range ps2016Preds {
			ps2016Check(t, s, p.f, p.name)
		}
	}
}

// TestEquiv_PS2016_AfterIsZeroAlloc pins the perf premise: the rewritten
// forms allocate nothing (they return a substring of the input), which
// is exactly the win the check promises. The Before-form's copies are
// measured in benchmarks/ps2016_test.go rather than asserted here, since
// escape analysis could legitimately elide them in some shapes.
func TestEquiv_PS2016_AfterIsZeroAlloc(t *testing.T) {
	s := "  \t a trimmed payload that is clearly long enough to heap-allocate \t  "
	var sink string
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.TrimFunc(s, unicode.IsSpace)
		sink = strings.TrimLeftFunc(s, unicode.IsSpace)
		sink = strings.TrimRightFunc(s, unicode.IsSpace)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the strings TrimFunc family allocated %v times per run; the zero-copy premise of PS2016 no longer holds", allocs)
	}
}
