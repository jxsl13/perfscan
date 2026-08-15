package checks

// Runtime differential for PS5034: strings.FieldsFunc(s, unicode.IsSpace)
// vs the strings.Fields(s) form the fix emits. The fix's safety
// argument: Fields is documented AND implemented as
// FieldsFunc(s, unicode.IsSpace) — its non-ASCII branch literally
// returns that call (identical by construction), and its ASCII fast
// path (taken only when every byte is below utf8.RuneSelf, hence valid
// UTF-8) slices out the same non-empty maximal runs of non-asciiSpace
// bytes, where asciiSpace equals unicode.IsSpace restricted to ASCII.
// Both sides allocate the result exactly (make([]string, fieldCount)),
// so len, cap, non-nil-ness, element order, and element bytes agree on
// every input, including "" and all-whitespace (both yield an empty
// NON-nil slice). This suite pins that claim over:
//
//   - EXHAUSTIVE inputs: every string of length <= 4 over an
//     adversarial alphabet — ASCII space and tab, letters, NUL, the
//     UTF-8 lead byte 0xC2, the continuation bytes 0xA0/0x85 (so NBSP
//     U+00A0 and NEL U+0085 — IsSpace-true non-ASCII runes — arise
//     in-sequence, and BARE continuation bytes arise as invalid UTF-8),
//     and 0xFF — truncated and misaligned sequences at every position;
//   - targeted seeds: "", single spaces, all-ASCII-whitespace runs,
//     every notable Unicode space rune back to back (NBSP, NEL, ogham
//     space mark, en/em spaces, line/paragraph separators, ideographic
//     space), fields glued to invalid bytes, a lone lead byte at EOF,
//     and RuneError itself in the input;
//   - randomized long inputs (up to 4 KiB) over the full byte range
//     with a fixed seed, plus mostly-ASCII inputs with rare multi-byte
//     spaces spliced in — long enough to exercise the ASCII counting
//     pass, the delegation branch, and large span-slice growth.
//
// Checked per input: len, cap, (a == nil) == (b == nil), and every
// element byte-for-byte.

import (
	"math/rand"
	"strings"
	"testing"
	"unicode"
)

// ps5034Before is the exact Before-shape of the check; ps5034After is
// the exact After-shape the fix emits. They must agree on EVERY input.
func ps5034Before(s string) []string { return strings.FieldsFunc(s, unicode.IsSpace) }
func ps5034After(s string) []string  { return strings.Fields(s) }

func ps5034Check(t *testing.T, s string) {
	t.Helper()
	before := ps5034Before(s)
	after := ps5034After(s)
	if (before == nil) != (after == nil) {
		t.Fatalf("nil-ness diverges for %q: FieldsFunc nil=%v, Fields nil=%v", s, before == nil, after == nil)
	}
	if len(before) != len(after) {
		t.Fatalf("len diverges for %q: FieldsFunc %d, Fields %d", s, len(before), len(after))
	}
	if cap(before) != cap(after) {
		t.Fatalf("cap diverges for %q: FieldsFunc %d, Fields %d", s, cap(before), cap(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("field %d diverges for %q: FieldsFunc %q, Fields %q", i, s, before[i], after[i])
		}
	}
}

// TestPS5034EquivalenceExhaustive crosses every string of length <= 4
// over the adversarial alphabet. NBSP (0xC2 0xA0) and NEL (0xC2 0x85)
// arise as adjacent pairs; bare 0xA0/0x85/0xFF and a trailing 0xC2 are
// invalid UTF-8 at every position.
func TestPS5034EquivalenceExhaustive(t *testing.T) {
	t.Parallel()
	alphabet := []byte{'a', 'Z', ' ', '\t', 0x00, 0xC2, 0xA0, 0x85, 0xFF}
	var rec func(prefix []byte, depth int)
	rec = func(prefix []byte, depth int) {
		ps5034Check(t, string(prefix))
		if depth == 0 {
			return
		}
		for _, b := range alphabet {
			rec(append(prefix, b), depth-1)
		}
	}
	rec(nil, 4)
}

// TestPS5034EquivalenceSeeds pins the documented edges directly.
func TestPS5034EquivalenceSeeds(t *testing.T) {
	t.Parallel()
	seeds := []string{
		"",
		" ",
		"\t\n\v\f\r ",
		"a",
		"  leading",
		"trailing  ",
		"one two\tthree\nfour",
		// every notable Unicode space back to back: NBSP, NEL, ogham
		// space mark, en space, em space, line separator, paragraph
		// separator, ideographic space
		"\u00a0\u0085\u1680\u2002\u2003\u2028\u2029\u3000",
		"a\u00a0b\u3000c",               // non-ASCII space separators
		"x\u0085y",                      // NEL between fields
		"h\u00e9llo w\u00f6rld",         // multi-byte non-space runes
		"a\xffb",                        // invalid byte inside a field
		"\xff \xfe",                     // invalid bytes as fields
		"a\xc2",                         // lone lead byte at EOF
		"\xc2 ",                         // lead byte then ASCII space
		"a\x80b \x80",                   // bare continuation bytes
		"\ufffd \ufffd",                 // RuneError itself (not a space)
		"\x00 \x00\x00 a",               // NUL-bearing fields
		"\u65e5\u672c\u3000\u8a9e test", // ideographic space between CJK fields
		strings.Repeat(" a", 100),       // many single-rune fields
		strings.Repeat("\u00a0a", 100),  // many NBSP-separated fields
		strings.Repeat("word ", 500),    // long mostly-ASCII
	}
	for _, s := range seeds {
		ps5034Check(t, s)
	}
}

// TestPS5034EquivalenceRandom drives seeded-random long inputs: full
// byte range (invalid UTF-8 everywhere) and mostly-ASCII with rare
// multi-byte spaces spliced in (the delegation boundary).
func TestPS5034EquivalenceRandom(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(0x5034))
	spaces := []string{" ", "\t", "\n", "\u00a0", "\u0085", "\u2003", "\u3000"}
	for i := 0; i < 300; i++ {
		n := rng.Intn(4096)
		raw := make([]byte, n)
		rng.Read(raw)
		ps5034Check(t, string(raw))

		var b strings.Builder
		for b.Len() < n {
			word := make([]byte, 1+rng.Intn(8))
			for j := range word {
				word[j] = byte('a' + rng.Intn(26))
			}
			b.Write(word)
			b.WriteString(spaces[rng.Intn(len(spaces))])
		}
		ps5034Check(t, b.String())
	}
}
