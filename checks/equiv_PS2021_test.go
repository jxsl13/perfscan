package checks

// Runtime differential for PS2021: parts := strings.Split(s, sep);
// parts[len(parts)-1] vs s[strings.LastIndexByte(s, c)+1:] for a
// SINGLE-BYTE separator. The safety argument: a one-byte needle can never
// self-overlap, so the boundary of Split's final piece — the last
// non-overlapping occurrence found scanning left-to-right — is exactly the
// rightmost occurrence LastIndexByte finds; when the separator does not
// occur, LastIndexByte returns -1 and s[0:] == s == Split's sole element,
// and both Split and LastIndexByte match RAW bytes with no rune
// interpretation, so separator bytes >= 0x80 (including ones landing
// mid-rune in invalid UTF-8) behave identically. The suite covers:
//
//   - EXHAUSTIVE short inputs: every s of length <= 6 over an adversarial
//     alphabet (a separator candidate, a plain byte, the two bytes of a
//     multi-byte rune so truncated and complete sequences both arise, and
//     0xFF which is never valid UTF-8), crossed with single-byte
//     separators covering ASCII, NUL, a rune-fragment byte and 0xFF;
//   - targeted seeds: the empty string, a lone separator, leading,
//     trailing and consecutive separators, no-occurrence inputs, long
//     separator-dense inputs and separator-free tails;
//   - randomized long inputs over the full byte range and over a tiny
//     separator-dense alphabet with a fixed seed;
//   - the divergence shapes the single-byte gate exists for: a multi-byte
//     separator ("aaa"/"aa": Split's final piece is "a" but the LastIndex
//     boundary yields "") and the empty separator (Split rune-explodes,
//     the reslice s[LastIndex(s, "")+1:] panics). If either ever stops
//     diverging, the gate is no longer load-bearing and should be
//     revisited.

import (
	"math/rand"
	"strings"
	"testing"
)

// ps2021Before is the matched shape, verbatim.
func ps2021Before(s, sep string) string {
	parts := strings.Split(s, sep)
	return parts[len(parts)-1]
}

// ps2021After is the rewrite for a single-byte separator.
func ps2021After(s string, c byte) string {
	return s[strings.LastIndexByte(s, c)+1:]
}

func ps2021Check(t *testing.T, s string, c byte) {
	t.Helper()
	// NOTE: the one-byte separator STRING is built with string([]byte{c}) —
	// string(c) would be a rune conversion and UTF-8-encode bytes >= 0x80
	// into TWO bytes. The check never takes this path (it reads the byte
	// out of the source's string constant), but the harness must not
	// either.
	sep := string([]byte{c})
	before := ps2021Before(s, sep)
	after := ps2021After(s, c)
	if before != after {
		t.Fatalf("divergence on s=%q sep=%q:\n Split last = %q\n LastIndexByte reslice = %q",
			s, sep, before, after)
	}
}

// ps2021Alphabet anchors separator hits (','), plain bytes ('a'), the two
// bytes of é (0xC3 0xA9) so truncated and complete sequences both arise,
// and 0xFF, which is never valid UTF-8.
var ps2021Alphabet = []byte{',', 'a', 0xC3, 0xA9, 0xFF}

// ps2021Seps covers ASCII separators (present in and absent from the
// alphabet), NUL, a rune-fragment byte and an invalid-UTF-8 byte.
var ps2021Seps = []byte{',', 'a', 'z', 0x00, 0xC3, 0xA9, 0xFF}

func TestEquiv_PS2021_ExhaustiveShort(t *testing.T) {
	const maxLen = 6
	buf := make([]byte, maxLen)
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			s := string(buf[:n])
			for _, c := range ps2021Seps {
				ps2021Check(t, s, c)
			}
			return
		}
		for _, b := range ps2021Alphabet {
			buf[depth] = b
			rec(n, depth+1)
		}
	}
	for n := 0; n <= maxLen; n++ {
		rec(n, 0)
	}
}

func TestEquiv_PS2021_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("field-", 4096)
	type seed struct {
		s string
		c byte
	}
	seeds := []seed{
		{"", ','},                       // empty input: Split gives [""], reslice gives ""
		{",", ','},                      // lone separator: last field is ""
		{",a", ','},                     // leading separator
		{"a,", ','},                     // trailing separator: last field is ""
		{"a,,b", ','},                   // consecutive separators
		{",,,,", ','},                   // separators only
		{"abc", ','},                    // no occurrence: whole input
		{"a,b,c,d,e", ','},              // the doc's canonical shape
		{"héllo,wörld,日本", ','},         // multibyte fields
		{"日本語", 0x9E},                   // separator byte inside a rune: raw-byte match
		{"a\x00b\x00c", 0x00},           // NUL separator
		{"a\xffb\xffc", 0xFF},           // invalid-UTF-8 separator
		{"\xc3(\xc3(", 0xC3},            // truncated-rune separator byte
		{long, '-'},                     // long, separator-dense
		{long + "tail", 'q'},            // long, no occurrence
		{"x" + long, '-'},               // long with separator-free head
		{strings.Repeat(",", 512), ','}, // long input of only separators
	}
	for _, sd := range seeds {
		ps2021Check(t, sd.s, sd.c)
	}
}

func TestEquiv_PS2021_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25082021))
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
	// Full byte range: mostly invalid UTF-8 at random alignments, with a
	// random separator byte per case.
	for range 100000 {
		s := gen(full, 0, 64)
		ps2021Check(t, s, byte(rng.Intn(256)))
	}
	// Tiny alphabet so separator hits are dense and adjacent.
	tiny := []byte{',', 'a', 0xC3, 0xA9}
	for range 100000 {
		s := gen(tiny, 0, 32)
		ps2021Check(t, s, tiny[rng.Intn(len(tiny))])
	}
}

// TestEquiv_PS2021_MultiByteSepDiverges pins the divergence the
// single-byte gate exists for: a two-byte separator can self-overlap, so
// Split's final-piece boundary (last NON-OVERLAPPING occurrence found
// left-to-right) differs from the rightmost occurrence LastIndex finds.
// Split("aaa", "aa") = ["", "a"] with final piece "a", while
// LastIndex("aaa", "aa") = 1 puts the boundary at 1+2 = 3, yielding "".
func TestEquiv_PS2021_MultiByteSepDiverges(t *testing.T) {
	s, sep := "aaa", "aa"
	split := ps2021Before(s, sep)
	resliced := s[strings.LastIndex(s, sep)+len(sep):]
	if split == resliced {
		t.Fatalf("expected the multi-byte separator %q on %q to diverge, both forms returned %q — the single-byte gate may be obsolete", sep, s, split)
	}
}

// TestEquiv_PS2021_EmptySepDiverges pins the other divergence shape: the
// empty separator makes Split rune-explode (final element = last rune),
// while LastIndex(s, "") == len(s) makes the reslice s[len(s)+1:] panic.
func TestEquiv_PS2021_EmptySepDiverges(t *testing.T) {
	s := "ab"
	if got := ps2021Before(s, ""); got != "b" {
		t.Fatalf("Split(%q, \"\") last element = %q, want %q", s, got, "b")
	}
	defer func() {
		if recover() == nil {
			t.Fatalf("expected s[strings.LastIndex(s, \"\")+1:] to panic on s=%q — the empty-separator gate may be obsolete", s)
		}
	}()
	_ = s[strings.LastIndex(s, "")+1:]
}
