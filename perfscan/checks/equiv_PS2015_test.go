package checks

// Runtime differential for PS2015: strings.Join(strings.Split(s, sep), new)
// vs strings.ReplaceAll(s, sep, new) for a NON-EMPTY separator. The fix's
// safety argument is the Split/Join inverse identity: Split cuts s at
// exactly the non-overlapping occurrences of sep found left-to-right via
// Index (byte-oriented), Join re-inserts new between the fragments, and
// Replace finds the SAME non-overlapping occurrences with the same Index
// scan and substitutes new at each — so p0+new+p1+...+new+pk is
// byte-for-byte ReplaceAll's output. This suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every s of length <= 5 over an adversarial
//     alphabet (ASCII, the two bytes of a multi-byte rune so truncated and
//     complete sequences both arise, and 0xFF which is never valid UTF-8),
//     crossed with non-empty separators of 1 and 2 bytes including rune
//     fragments and invalid bytes, and replacements that are empty, equal
//     to the separator, longer than it, and invalid UTF-8 — every
//     alignment of matching, adjacent and boundary-straddling occurrences
//     the scan can see;
//   - targeted seeds: long inputs, separator-only inputs, zero-match
//     inputs, NUL separators, CJK separators, sep == new (identity
//     round-trip), overlapping-candidate separators ("aa" over "aaaa");
//   - randomized long inputs over the full byte range and over a tiny
//     separator-dense alphabet with a fixed seed;
//   - the divergence shape the constant-non-empty gate exists for:
//     sep == "" makes Split rune-explode and Join fill the k-1 gaps while
//     ReplaceAll inserts new k+1 times — if that ever stops diverging, the
//     gate is no longer load-bearing and should be revisited.

import (
	"math/rand"
	"strings"
	"testing"
)

// ps2015Check pins Join(Split(s, sep), new) == ReplaceAll(s, sep, new)
// for a non-empty separator.
func ps2015Check(t *testing.T, s, sep, repl string) {
	t.Helper()
	if sep == "" {
		t.Fatal("test bug: sep must be non-empty — the empty separator is the divergence shape, never matched by PS2015")
	}
	joined := strings.Join(strings.Split(s, sep), repl)
	replaced := strings.ReplaceAll(s, sep, repl)
	if joined != replaced {
		t.Fatalf("divergence on s=%q sep=%q new=%q:\n Join(Split)=%q\n ReplaceAll=%q",
			s, sep, repl, joined, replaced)
	}
}

// ps2015Alphabet anchors plain matches ('a','b'), straddles rune
// boundaries (0xC3 0xA9 are the bytes of é) and includes 0xFF, which is
// never valid UTF-8 — so separator matches can cut runes apart at every
// alignment.
var ps2015Alphabet = []byte{'a', 'b', 0xC3, 0xA9, 0xFF}

func ps2015ShortStrings(maxLen int) []string {
	var strs []string
	buf := make([]byte, maxLen)
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			strs = append(strs, string(buf[:n]))
			return
		}
		for _, c := range ps2015Alphabet {
			buf[depth] = c
			rec(n, depth+1)
		}
	}
	for n := 0; n <= maxLen; n++ {
		rec(n, 0)
	}
	return strs
}

func TestEquiv_PS2015_ExhaustiveShort(t *testing.T) {
	strs := ps2015ShortStrings(5)
	// "a" and "aa" exercise dense and adjacent (overlap-candidate)
	// matches over the same inputs; "\xC3" is a rune fragment and "\xFF"
	// an invalid byte, so matches can cut runes apart; "é" is a complete
	// 2-byte rune whose bytes also appear separately in the alphabet.
	seps := []string{"a", "aa", "ab", "\xC3", "\xFF", "é"}
	// Empty (deletion), 1-byte, separator-equal, longer-than-sep and
	// invalid-UTF-8 replacements.
	repls := []string{"", ";", "a", "; ", "\xFF\xC3", "é"}
	for _, s := range strs {
		for _, sep := range seps {
			for _, repl := range repls {
				ps2015Check(t, s, sep, repl)
			}
		}
	}
}

func TestEquiv_PS2015_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("field-", 4096)
	type seed struct {
		s, sep, repl string
	}
	seeds := []seed{
		{"a,b,c,d,e", ",", "; "}, // the doc's Before/After
		{"", ",", ";"},           // empty input: one empty piece, zero matches
		{"a", ",", ";"},          // no match: single piece round-trip
		{"no-separator-here at all", "MISSING", "x"},    // zero matches on a long input
		{",,,,", ",", ";"},                              // separators only: empty fields everywhere
		{",a,", ",", "::"},                              // leading and trailing separators
		{"aaaa", "aa", "b"},                             // overlapping candidates: non-overlapping left-to-right match
		{"aaaaa", "aa", "b"},                            // odd tail after adjacent matches
		{"a,b,c", ",", ","},                             // sep == new: identity round-trip
		{"héllo,wörld,日本,語", ",", "、"},                  // multibyte fields, multibyte replacement
		{"日本語日本語", "本", "ほん"},                           // CJK separator
		{"a\xFFb\xFFc", "\xFF", "?"},                    // invalid-UTF-8 separator
		{"\xC3\x28\xC3\x28", "\xC3", ""},                // truncated-rune separator, deleting
		{"a\x00b\x00c", "\x00", " "},                    // NUL separator
		{strings.Repeat("é", 64), "é", "e"},             // input consists ONLY of separators
		{"x" + long + "x", "-", "_"},                    // long, separator-dense
		{long + long, "field", "FIELD"},                 // separator == field text, longer replacement
		{strings.Repeat("　", 64) + "end", "　", "."},     // long multibyte separator run
		{"tail-sep-grows", "-", strings.Repeat("+", 8)}, // replacement much longer than separator
	}
	for _, sd := range seeds {
		ps2015Check(t, sd.s, sd.sep, sd.repl)
	}
}

func TestEquiv_PS2015_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25082015))
	gen := func(al []byte, min, max int) string {
		b := make([]byte, min+rng.Intn(max-min+1))
		for i := range b {
			b[i] = al[rng.Intn(len(al))]
		}
		return string(b)
	}
	full := make([]byte, 256)
	for i := range full {
		full[i] = byte(i)
	}
	// Full byte range: mostly invalid UTF-8 at random alignments.
	for range 50000 {
		s := gen(full, 0, 48)
		sep := gen(full, 1, 3) // non-empty by construction
		repl := gen(full, 0, 4)
		ps2015Check(t, s, sep, repl)
	}
	// Tiny alphabet so separator matches are dense and adjacent, and the
	// replacement often equals or contains the separator.
	tiny := []byte{'a', 'b', 0xC3, 0xA9}
	for range 50000 {
		s := gen(tiny, 0, 32)
		sep := gen(tiny, 1, 2)
		repl := gen(tiny, 0, 3)
		ps2015Check(t, s, sep, repl)
	}
}

// TestEquiv_PS2015_EmptySepDiverges pins the divergence the
// constant-non-empty gate exists for: Split(s, "") explodes s into its k
// runes and Join fills the k-1 gaps, while ReplaceAll(s, "", new)
// matches the empty string before and after every rune and inserts new
// k+1 times. If this ever stops diverging, the gate is no longer
// load-bearing and the check should be revisited.
func TestEquiv_PS2015_EmptySepDiverges(t *testing.T) {
	s, repl := "ab", ";"
	joined := strings.Join(strings.Split(s, ""), repl) // "a;b"
	replaced := strings.ReplaceAll(s, "", repl)        // ";a;b;"
	if joined == replaced {
		t.Fatalf("expected Join(Split(s, %q), %q) and ReplaceAll(s, %q, %q) to diverge on s=%q, but both returned %q — the non-empty-separator gate may be obsolete",
			"", repl, "", repl, s, joined)
	}
	// Even the empty input diverges: Split("", "") has ZERO pieces so the
	// join is "", while ReplaceAll("", "", new) inserts new once.
	if got := strings.Join(strings.Split("", ""), repl); got == strings.ReplaceAll("", "", repl) {
		t.Fatalf("expected divergence on the empty input with the empty separator, both returned %q", got)
	}
}
