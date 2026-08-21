package checks

// Runtime differential for PS3030: bytes.FieldsFunc(b, unicode.IsSpace)
// vs the bytes.Fields(b) form the fix emits. The fix's safety argument:
// Fields is documented AND implemented as FieldsFunc(s, unicode.IsSpace)
// — its non-ASCII branch literally returns that call (identical by
// construction), and its ASCII fast path (taken only when every byte is
// below utf8.RuneSelf, hence valid UTF-8) slices out the same non-empty
// maximal runs of non-asciiSpace bytes, where asciiSpace equals
// unicode.IsSpace restricted to ASCII. Both sides return
// three-index-capped subslices of the SAME backing array (Fields:
// s[fieldStart:i:i] / s[fieldStart:len(s):len(s)]; FieldsFunc:
// s[span.start:span.end:span.end]) and allocate the outer slice exactly
// (make([][]byte, fieldCount)), so len, cap, non-nil-ness (nil, empty,
// and all-whitespace all yield an empty NON-nil slice), element order,
// element bytes, per-field cap, and per-field backing-array identity —
// the aliasing surface that matters for a mutable []byte — agree on
// every input. This suite pins that claim over:
//
//   - EXHAUSTIVE inputs: every byte slice of length <= 4 over an
//     adversarial alphabet — ASCII space and tab, letters, NUL, the
//     UTF-8 lead byte 0xC2, the continuation bytes 0xA0/0x85 (so NBSP
//     U+00A0 and NEL U+0085 — IsSpace-true non-ASCII runes — arise
//     in-sequence, and BARE continuation bytes arise as invalid UTF-8),
//     and 0xFF — truncated and misaligned sequences at every position;
//   - targeted seeds: nil, empty, single spaces, all-ASCII-whitespace
//     runs, every notable Unicode space rune back to back (NBSP, NEL,
//     ogham space mark, en/em spaces, line/paragraph separators,
//     ideographic space), fields glued to invalid bytes, a lone lead
//     byte at EOF, and RuneError itself in the input;
//   - randomized long inputs (up to 4 KiB) over the full byte range
//     with a fixed seed, plus mostly-ASCII inputs with rare multi-byte
//     spaces spliced in — long enough to exercise the ASCII counting
//     pass, the delegation branch, and large span-slice growth.
//
// Checked per input: outer len, cap, (a == nil) == (b == nil), and per
// field: bytes, cap, and base-pointer identity into the shared input.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unicode"
)

// ps3030Before is the exact Before-shape of the check; ps3030After is
// the exact After-shape the fix emits. They must agree on EVERY input.
func ps3030Before(b []byte) [][]byte { return bytes.FieldsFunc(b, unicode.IsSpace) }
func ps3030After(b []byte) [][]byte  { return bytes.Fields(b) }

func ps3030Check(t *testing.T, b []byte) {
	t.Helper()
	before := ps3030Before(b)
	after := ps3030After(b)
	if (before == nil) != (after == nil) {
		t.Fatalf("nil-ness diverges for %q: FieldsFunc nil=%v, Fields nil=%v", b, before == nil, after == nil)
	}
	if len(before) != len(after) {
		t.Fatalf("len diverges for %q: FieldsFunc %d, Fields %d", b, len(before), len(after))
	}
	if cap(before) != cap(after) {
		t.Fatalf("cap diverges for %q: FieldsFunc %d, Fields %d", b, cap(before), cap(after))
	}
	for i := range before {
		if !bytes.Equal(before[i], after[i]) {
			t.Fatalf("field %d diverges for %q: FieldsFunc %q, Fields %q", i, b, before[i], after[i])
		}
		if cap(before[i]) != cap(after[i]) {
			t.Fatalf("field %d cap diverges for %q: FieldsFunc %d, Fields %d", i, b, cap(before[i]), cap(after[i]))
		}
		// Fields are documented non-empty on both sides; pin that AND
		// that both alias the SAME position of the shared input — the
		// mutation-visibility surface a []byte caller can observe.
		if len(before[i]) == 0 || len(after[i]) == 0 {
			t.Fatalf("field %d empty for %q: FieldsFunc %q, Fields %q", i, b, before[i], after[i])
		}
		if &before[i][0] != &after[i][0] {
			t.Fatalf("field %d backing pointer diverges for %q", i, b)
		}
	}
}

// TestPS3030EquivalenceExhaustive crosses every byte slice of length
// <= 4 over the adversarial alphabet. NBSP (0xC2 0xA0) and NEL
// (0xC2 0x85) arise as adjacent pairs; bare 0xA0/0x85/0xFF and a
// trailing 0xC2 are invalid UTF-8 at every position.
func TestPS3030EquivalenceExhaustive(t *testing.T) {
	t.Parallel()
	alphabet := []byte{'a', 'Z', ' ', '\t', 0x00, 0xC2, 0xA0, 0x85, 0xFF}
	var rec func(prefix []byte, depth int)
	rec = func(prefix []byte, depth int) {
		// Clone so each probe owns its backing array: the pointer
		// identity check compares positions WITHIN one shared input.
		ps3030Check(t, bytes.Clone(prefix))
		if depth == 0 {
			return
		}
		for _, c := range alphabet {
			rec(append(prefix, c), depth-1)
		}
	}
	rec(nil, 4)
}

// TestPS3030EquivalenceSeeds pins the documented edges directly,
// including the nil slice (bytes-specific: still an empty NON-nil
// result on both sides).
func TestPS3030EquivalenceSeeds(t *testing.T) {
	t.Parallel()
	seeds := [][]byte{
		nil,
		{},
		[]byte(" "),
		[]byte("\t\n\v\f\r "),
		[]byte("a"),
		[]byte("  leading"),
		[]byte("trailing  "),
		[]byte("one two\tthree\nfour"),
		// every notable Unicode space back to back: NBSP, NEL, ogham
		// space mark, en space, em space, line separator, paragraph
		// separator, ideographic space
		[]byte("\u00a0\u0085\u1680\u2002\u2003\u2028\u2029\u3000"),
		[]byte("a\u00a0b\u3000c"),               // non-ASCII space separators
		[]byte("x\u0085y"),                      // NEL between fields
		[]byte("h\u00e9llo w\u00f6rld"),         // multi-byte non-space runes
		[]byte("a\xffb"),                        // invalid byte inside a field
		[]byte("\xff \xfe"),                     // invalid bytes as fields
		[]byte("a\xc2"),                         // lone lead byte at EOF
		[]byte("\xc2 "),                         // lead byte then ASCII space
		[]byte("a\x80b \x80"),                   // bare continuation bytes
		[]byte("\ufffd \ufffd"),                 // RuneError itself (not a space)
		[]byte("\x00 \x00\x00 a"),               // NUL-bearing fields
		[]byte("\u65e5\u672c\u3000\u8a9e test"), // ideographic space between CJK fields
		[]byte(strings.Repeat(" a", 100)),       // many single-rune fields
		[]byte(strings.Repeat("\u00a0a", 100)),  // many NBSP-separated fields
		[]byte(strings.Repeat("word ", 500)),    // long mostly-ASCII
	}
	for _, b := range seeds {
		ps3030Check(t, b)
	}
}

// TestPS3030EquivalenceRandom drives seeded-random long inputs: full
// byte range (invalid UTF-8 everywhere) and mostly-ASCII with rare
// multi-byte spaces spliced in (the delegation boundary).
func TestPS3030EquivalenceRandom(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(0x3030))
	spaces := []string{" ", "\t", "\n", "\u00a0", "\u0085", "\u2003", "\u3000"}
	for i := 0; i < 300; i++ {
		n := rng.Intn(4096)
		raw := make([]byte, n)
		rng.Read(raw)
		ps3030Check(t, raw)

		var b bytes.Buffer
		for b.Len() < n {
			word := make([]byte, 1+rng.Intn(8))
			for j := range word {
				word[j] = byte('a' + rng.Intn(26))
			}
			b.Write(word)
			b.WriteString(spaces[rng.Intn(len(spaces))])
		}
		ps3030Check(t, b.Bytes())
	}
}
