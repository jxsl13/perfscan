package checks

// Runtime differential for PS2025: utf8.Valid([]byte(s)) vs
// utf8.ValidString(s), and the symmetric reverse
// utf8.ValidString(string(b)) vs utf8.Valid(b). The fix's safety argument
// is that Valid and ValidString implement one validation algorithm twice
// — the same word-at-a-time ASCII fast path, the same first/acceptRanges
// tables, the same per-sequence bounds checks — and the conversion
// preserves byte content exactly, so the returned bool is equal for every
// input. This suite pins that claim over:
//
//   - EXHAUSTIVE inputs: every byte string of length <= 3 over an
//     adversarial alphabet (ASCII, NUL, every byte of a two-byte and a
//     three-byte rune so truncated, complete, and misaligned sequences
//     arise at every position, the four-byte starter 0xF0, a bare
//     continuation byte, the overlong/surrogate starters 0xC0/0xC1/0xED,
//     the out-of-range starter 0xF5, and 0xFF) — every valid and invalid
//     shape of up to three bytes arises from the cross product;
//   - targeted seeds: the empty input, lone and paired surrogates
//     (ED A0 80 / ED BF BF), overlong encodings (C0 80, E0 80 80,
//     F0 80 80 80), the maximal rune F4 8F BF BF and its out-of-range
//     successor F4 90 80 80, 0xFE/0xFF, a truncated sequence at the very
//     end of a long ASCII run (crossing the word-at-a-time fast path),
//     and U+FFFD itself (a REPLACEMENT CHARACTER already in the input
//     stays valid);
//   - randomized long inputs over the full byte range and over
//     valid-rune-biased content with a fixed seed.
//
// It also pins the perf premise on the After shapes: utf8.ValidString(s)
// and utf8.Valid(b) allocate nothing, so the entire Before-side cost is
// the conversion's copy and (past the stack temporary) its allocation.

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// ps2025Before* are the exact Before-shapes of the check: validation fed
// through a throwaway conversion.
func ps2025BeforeForward(s string) bool { return utf8.Valid([]byte(s)) }
func ps2025BeforeReverse(b []byte) bool { return utf8.ValidString(string(b)) }

// ps2025After* are the exact After-shapes of the check: the sibling
// function over the operand itself.
func ps2025AfterForward(s string) bool { return utf8.ValidString(s) }
func ps2025AfterReverse(b []byte) bool { return utf8.Valid(b) }

// ps2025Check pins both directions on one byte content: the forward
// rewrite on string(content) and the reverse rewrite on the []byte —
// and cross-checks all four spellings against each other, so the two
// stdlib implementations may never disagree on any content either.
func ps2025Check(t *testing.T, content []byte) {
	t.Helper()
	s := string(content)
	want := ps2025AfterForward(s)
	if got := ps2025BeforeForward(s); got != want {
		t.Fatalf("forward divergence on %q: utf8.Valid([]byte(s))=%v utf8.ValidString(s)=%v", s, got, want)
	}
	if got := ps2025AfterReverse(content); got != want {
		t.Fatalf("cross divergence on %q: utf8.Valid(b)=%v utf8.ValidString(s)=%v", s, got, want)
	}
	if got := ps2025BeforeReverse(content); got != want {
		t.Fatalf("reverse divergence on %q: utf8.ValidString(string(b))=%v utf8.Valid(b)=%v", s, got, want)
	}
}

// ps2025Alphabet stresses every validation hazard: ASCII, NUL, the bytes
// of the two-byte U+00E9 (C3 A9) and three-byte U+3042 (E3 81 82) so
// truncated and misaligned sequences arise at every position, the
// four-byte starter 0xF0, a bare continuation byte 0x80, the overlong
// starters 0xC0/0xC1, the surrogate starter 0xED, the out-of-range
// starter 0xF5, and 0xFF (never valid UTF-8).
var ps2025Alphabet = []byte{'a', 0x00, 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xF0, 0x80, 0xC0, 0xC1, 0xED, 0xF5, 0xFF}

func TestEquiv_PS2025_ExhaustiveShort(t *testing.T) {
	var enumerate func(buf []byte, depth int)
	enumerate = func(buf []byte, depth int) {
		if depth == len(buf) {
			ps2025Check(t, buf)
			return
		}
		for _, c := range ps2025Alphabet {
			buf[depth] = c
			enumerate(buf, depth+1)
		}
	}
	for n := 0; n <= 3; n++ {
		enumerate(make([]byte, n), 0)
	}
}

func TestEquiv_PS2025_TargetedSeeds(t *testing.T) {
	seeds := [][]byte{
		nil,
		{},
		[]byte("plain ascii only"),
		[]byte("héllo, 世界 — ✓ 🚀"),            // 2-, 3-, and 4-byte runes
		{0xED, 0xA0, 0x80},                   // lone high surrogate U+D800
		{0xED, 0xBF, 0xBF},                   // lone low surrogate U+DFFF
		{0xED, 0xA0, 0x80, 0xED, 0xBF, 0xBF}, // surrogate pair, CESU-8 style
		{0xC0, 0x80},                         // overlong NUL
		{0xC1, 0xBF},                         // overlong U+007F
		{0xE0, 0x80, 0x80},                   // overlong 3-byte
		{0xF0, 0x80, 0x80, 0x80},             // overlong 4-byte
		{0xF4, 0x8F, 0xBF, 0xBF},             // U+10FFFF, the maximal rune
		{0xF4, 0x90, 0x80, 0x80},             // one past the maximal rune
		{0xFE}, {0xFF}, {0xFE, 0xFF},         // never-valid bytes
		{0x80},                            // bare continuation byte
		{0xC3},                            // truncated 2-byte sequence
		{0xE3, 0x81},                      // truncated 3-byte sequence
		{0xF0, 0x9F, 0x9A},                // truncated 4-byte sequence
		[]byte("�"),                       // U+FFFD itself is valid input
		[]byte(strings.Repeat("x", 4096)), // long pure-ASCII fast path
		append([]byte(strings.Repeat("x", 4093)), 0xE3, 0x81), // truncation at the end of a long ASCII run
		append([]byte(strings.Repeat("x", 63)), 0x80),         // continuation byte right after the word-at-a-time blocks
		[]byte(strings.Repeat("héllo, 世界! 🚀 ", 128)),          // long mixed-width valid input
	}
	for _, seed := range seeds {
		ps2025Check(t, seed)
	}
}

func TestEquiv_PS2025_Randomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25))
	for range 300 {
		// Full byte range: overwhelmingly invalid inputs, with valid
		// prefixes of every length before the first bad sequence.
		b := make([]byte, rng.Intn(512))
		rng.Read(b)
		ps2025Check(t, b)

		// Valid-rune-biased content with occasional raw byte damage, so
		// long valid runs meet rare, isolated invalid sequences.
		var sb strings.Builder
		for sb.Len() < 256 {
			if rng.Intn(16) == 0 {
				sb.WriteByte(byte(rng.Intn(256)))
				continue
			}
			sb.WriteRune(rune(rng.Intn(0x110000)))
		}
		ps2025Check(t, []byte(sb.String()))
	}
}

// TestEquiv_PS2025_AfterAllocFree pins the perf premise: both After
// shapes are allocation-free, so the Before sides' conversion copy (and,
// past gc's 32-byte stack temporary, its heap allocation) is the entire
// delta the rewrite removes.
func TestEquiv_PS2025_AfterAllocFree(t *testing.T) {
	s := strings.Repeat("héllo, 世界! ", 8)
	b := []byte(s)
	var sink bool
	if avg := testing.AllocsPerRun(100, func() { sink = ps2025AfterForward(s) }); avg != 0 {
		t.Errorf("utf8.ValidString allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sink = ps2025AfterReverse(b) }); avg != 0 {
		t.Errorf("utf8.Valid allocates %v times per run, want 0", avg)
	}
	_ = sink
}
