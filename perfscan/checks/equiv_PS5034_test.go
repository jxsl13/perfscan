package checks

// Runtime differential for PS5034: strings.TrimFunc(s, unicode.IsSpace)
// vs strings.TrimSpace(s). The fix's safety argument: strings.TrimSpace
// is DEFINED as the trim over unicode.IsSpace — its implementation
// literally delegates to TrimFunc(s[lo:], unicode.IsSpace) (resp.
// TrimRightFunc) the moment its ASCII scan meets a byte >=
// utf8.RuneSelf, and its fast-path table asciiSpace marks exactly
// {'\t', '\n', '\v', '\f', '\r', ' '}, which equals unicode.IsSpace
// restricted to single bytes below utf8.RuneSelf (the other Latin-1
// spaces U+0085 and U+00A0 are multi-byte in UTF-8, so they reach the
// shared rune path in both spellings; a RAW 0x85/0xA0 byte is invalid
// UTF-8, decodes to utf8.RuneError in both, and RuneError is not a
// space). Both forms return a substring of s with identical start/stop
// bounds, and strings are immutable, so byte equality is the whole
// observable story. This suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 4 over an
//     adversarial alphabet (each ASCII white-space byte class, a
//     non-space ASCII byte, NUL, the bytes of NBSP U+00A0 and NEL
//     U+0085 and the multi-byte OGHAM SPACE MARK U+1680 and IDEOGRAPHIC
//     SPACE U+3000 — so truncated and complete sequences arise at every
//     position — a bare continuation byte, and 0xFF), which covers
//     all-space inputs, space-only edges, invalid UTF-8 at both
//     boundaries, and interior white space that must NOT be trimmed;
//   - targeted seeds: empty, all-ASCII-space, every Unicode White_Space
//     rune as padding (U+0085, U+00A0, U+1680, U+2000..U+200A, U+2028,
//     U+2029, U+202F, U+205F, U+3000), white space only on one side,
//     RuneError-adjacent shapes (truncated NBSP, bare continuation,
//     0xFF at the boundary), the literal replacement char (NOT a
//     space in either spelling), and long inputs on and off the ASCII
//     fast path;
//   - randomized inputs with a fixed seed, biased toward boundary white
//     space, multi-byte space runes sliced mid-sequence, and invalid
//     bytes.
//
// It also pins the perf premise that NO side allocates: both return a
// substring of s.

import (
	"math/rand"
	"strings"
	"testing"
	"unicode"
)

// ps5034Before is the exact Before-shape of the check; ps5034After is
// the exact After-shape the fix emits. They must agree byte-for-byte on
// EVERY input.
func ps5034Before(s string) string { return strings.TrimFunc(s, unicode.IsSpace) }
func ps5034After(s string) string  { return strings.TrimSpace(s) }

func ps5034Check(t *testing.T, s string) {
	t.Helper()
	if before, after := ps5034Before(s), ps5034After(s); before != after {
		t.Fatalf("TrimSpace divergence on %q:\n before=%q\n after=%q", s, before, after)
	}
}

func TestEquiv_PS5034_ExhaustiveShort(t *testing.T) {
	alphabet := []byte{
		' ', '\t', '\n', '\v', '\f', '\r', // the asciiSpace table, complete
		'a',        // ASCII non-space
		0x00,       // NUL (not a space)
		0xC2, 0xA0, // C2 A0 = NBSP U+00A0: IsSpace-true, multi-byte
		0x85,             // second byte of NEL U+0085 (C2 85) and a bare invalid byte alone
		0xE1, 0x9A, 0x80, // E1 9A 80 = OGHAM SPACE MARK U+1680
		0xE3, 0x80, // truncated IDEOGRAPHIC SPACE U+3000 prefix (E3 80 80)
		0xFF, // never valid in UTF-8
	}
	var buf [4]byte
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			ps5034Check(t, string(buf[:n]))
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

func TestEquiv_PS5034_TargetedSeeds(t *testing.T) {
	// Every White_Space rune Go's unicode.IsSpace reports true for.
	spaces := []rune{
		'\t', '\n', '\v', '\f', '\r', ' ',
		0x85, 0xA0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005,
		0x2006, 0x2007, 0x2008, 0x2009, 0x200A,
		0x2028, 0x2029, 0x202F, 0x205F, 0x3000,
	}
	seeds := []string{
		"",
		" ",
		" \t\r\n\v\f ",              // all-ASCII-space: both sides yield ""
		"x",                         // nothing to trim
		"  leading only",            //
		"trailing only\t\t",         //
		"  both sides  ",            //
		" interior  space  kept ",   // interior runs must survive verbatim
		"\u00a0nbsp padded\u00a0",   // multi-byte space at both ends
		"\u0085nel\u0085",           //
		"\u3000ideographic\u3000",   //
		"\u200bZWSP is NOT a space", // U+200B: IsSpace-false in Go
		"\ufeffBOM is not a space",  //
		"�",                         // literal RuneError: not a space either way
		" �\t",                      // RuneError inside the trimmed region's edges
		"\xc2",                      // truncated NBSP at the very start
		"\xc2 ",                     //
		" \xc2",                     // truncated NBSP after trailing-space start
		"\x85",                      // bare NEL byte: invalid alone, decodes RuneError
		"\xa0\xa0",                  // bare continuation bytes only
		"\xffdata\xff",              // invalid bytes as hard boundaries
		" \xff ",                    // ASCII space around an invalid byte
		"a\x00b",                    // NUL inside
		" \u00a0 \t\u3000 mixed run of spaces \u2028\r\n ",
		strings.Repeat(" ", 4096),                       // long all-space
		strings.Repeat("x", 4096),                       // long no-trim ASCII fast path
		"  " + strings.Repeat("x", 4096) + "\t\t",       // long ASCII with trimmed edges
		"\u00a0" + strings.Repeat("x", 4096) + "\u00a0", // long input on the delegated rune path
		strings.Repeat("\u3000", 1024),                  // long all-unicode-space
		strings.Repeat(" x", 2048) + strings.Repeat(" ", 64),
	}
	for _, r := range spaces {
		seeds = append(seeds,
			string(r),
			string(r)+"core"+string(r),
			string(r)+string(r)+"x",
			"x"+string(r)+string(r),
			// The rune sliced to just its first byte on either boundary:
			// invalid UTF-8 that must NOT be trimmed.
			string(string(r)[:1])+"y"+string(string(r)[:1]),
		)
	}
	for _, s := range seeds {
		ps5034Check(t, s)
	}
}

func TestEquiv_PS5034_Randomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25034))
	spaceRunes := []rune{'\t', '\n', ' ', 0x85, 0xA0, 0x1680, 0x2003, 0x2028, 0x202F, 0x3000}
	pickers := []func() byte{
		func() byte { return byte(rng.Intn(128)) },        // ASCII
		func() byte { return " \t\r\n\v\f"[rng.Intn(6)] }, // ASCII spaces
		func() byte { // bytes of multi-byte space runes (sliced mid-rune on purpose)
			enc := string(spaceRunes[rng.Intn(len(spaceRunes))])
			return enc[rng.Intn(len(enc))]
		},
		func() byte { return byte(0x80 + rng.Intn(64)) }, // bare continuations
		func() byte { return byte(rng.Intn(256)) },       // anything
	}
	for range 200000 {
		pick := pickers[rng.Intn(len(pickers))]
		b := make([]byte, rng.Intn(25))
		for i := range b {
			b[i] = pick()
		}
		// Half the time, splice complete space runes onto the ends so
		// deep trims of valid multi-byte white space arise often.
		s := string(b)
		if rng.Intn(2) == 0 {
			for range rng.Intn(4) {
				s = string(spaceRunes[rng.Intn(len(spaceRunes))]) + s
			}
			for range rng.Intn(4) {
				s += string(spaceRunes[rng.Intn(len(spaceRunes))])
			}
		}
		ps5034Check(t, s)
	}
}

// TestEquiv_PS5034_NoAlloc pins the perf premise: neither spelling
// allocates — both return a substring of s.
func TestEquiv_PS5034_NoAlloc(t *testing.T) {
	s := "  \t padded payload with interior  spaces \u00a0 "
	var out string
	if avg := testing.AllocsPerRun(100, func() { out = ps5034Before(s) }); avg != 0 {
		t.Errorf("TrimFunc allocates: %v allocs/op", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { out = ps5034After(s) }); avg != 0 {
		t.Errorf("TrimSpace allocates: %v allocs/op", avg)
	}
	_ = out
}
