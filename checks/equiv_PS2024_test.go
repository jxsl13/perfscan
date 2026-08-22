package checks

// Runtime differential for PS2024: utf8.RuneCount([]byte(s)) vs
// utf8.RuneCountInString(s), and the mirror
// utf8.RuneCountInString(string(b)) vs utf8.RuneCount(b).
//
// The fix's safety argument is that unicode/utf8's two counters share
// one counting core — the standard library implements RuneCount's
// non-ASCII path directly in terms of RuneCountInString over the
// remaining bytes, and both count exactly one rune per byte of any
// invalid sequence — while []byte(s) has byte
// content identical to s (resp. string(b) to b, with
// string(nil []byte) == ""), so the integer results agree on EVERY
// input. This suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 4 over an
//     adversarial alphabet (NUL, plain ASCII, the last one-byte rune
//     0x7F, a bare continuation byte, the bytes of two- and three-byte
//     runes so truncated and complete sequences both arise, a four-byte
//     lead, the always-invalid lead 0xC0 that spells an overlong
//     encoding, and 0xFF);
//   - targeted seeds: empty, pure ASCII, mixed-width valid UTF-8, the
//     replacement char itself, a CESU-8-style encoded surrogate half
//     (invalid in Go), overlong encodings, a maximal four-byte rune,
//     out-of-range 0xF5 leads, truncated sequences at end of input, and
//     long strings crossing every internal chunk boundary;
//   - randomized long inputs over the full byte range with a fixed
//     seed.
//
// It also pins the perf premise: the string-direction After is
// allocation-free on every input, and the mirror After never allocates
// more than its Before — zero on ASCII-dominated inputs, at most
// RuneCount's internal tail copy (strictly no larger than the Before's
// full copy) otherwise.

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// ps2024Before/After are the exact Before/After shapes of the check.
func ps2024Before(s string) int       { return utf8.RuneCount([]byte(s)) }
func ps2024After(s string) int        { return utf8.RuneCountInString(s) }
func ps2024BeforeMirror(b []byte) int { return utf8.RuneCountInString(string(b)) }
func ps2024AfterMirror(b []byte) int  { return utf8.RuneCount(b) }

// ps2024Check verifies both directions on one input's byte content.
func ps2024Check(t *testing.T, s string) {
	t.Helper()
	if got, want := ps2024After(s), ps2024Before(s); got != want {
		t.Fatalf("string direction divergence on %q: RuneCount([]byte(s))=%d, RuneCountInString(s)=%d", s, want, got)
	}
	b := []byte(s)
	if got, want := ps2024AfterMirror(b), ps2024BeforeMirror(b); got != want {
		t.Fatalf("mirror direction divergence on %q: RuneCountInString(string(b))=%d, RuneCount(b)=%d", s, want, got)
	}
}

func TestPS2024EquivalenceExhaustive(t *testing.T) {
	// Adversarial alphabet: NUL, ASCII, the 0x7F boundary, a bare
	// continuation byte, é (0xC3 0xA9), the bytes of € (0xE2 0x82 0xAC),
	// a four-byte lead (0xF0), the always-invalid overlong lead 0xC0,
	// and 0xFF. Truncated, complete, and illegally-recombined sequences
	// all arise from the cross product.
	alphabet := []byte{0x00, 'a', 0x7F, 0x80, 0xC3, 0xA9, 0xE2, 0x82, 0xAC, 0xF0, 0xC0, 0xFF}
	var rec func(prefix []byte, depth int)
	rec = func(prefix []byte, depth int) {
		ps2024Check(t, string(prefix))
		if depth == 0 {
			return
		}
		for _, c := range alphabet {
			rec(append(prefix, c), depth-1)
		}
	}
	rec(nil, 4)
}

func TestPS2024EquivalenceSeeds(t *testing.T) {
	seeds := []string{
		"",
		"plain ascii only",
		"héllo wörld … é€\U0001F4A9",
		"�",                                  // the replacement char itself, validly encoded
		"\xed\xa0\x80",                       // encoded surrogate half — invalid in Go
		"\xc0\xaf",                           // overlong encoding of '/'
		"\xe0\x80\x80",                       // overlong three-byte encoding
		"\xf4\x8f\xbf\xbf",                   // U+10FFFF, the maximal rune
		"\xf5\x80\x80\x80",                   // lead beyond U+10FFFF
		"\xf0\x9f\x92",                       // truncated four-byte sequence at end
		"abc\xc3",                            // truncated two-byte sequence at end
		"\x80\x80\x80\x80",                   // bare continuation bytes
		"a\xffb\xfec",                        // 0xFF/0xFE interleaved with ASCII
		strings.Repeat("x", 4096),            // long pure ASCII
		strings.Repeat("héllo…", 1024),       // long mixed-width valid UTF-8
		strings.Repeat("\xc3x\xe2\x82", 512), // long invalid/truncated mix
	}
	for _, s := range seeds {
		ps2024Check(t, s)
	}
	// The mirror must also hold for a nil []byte: string(nil) == "".
	if got, want := ps2024AfterMirror(nil), ps2024BeforeMirror(nil); got != want {
		t.Fatalf("mirror divergence on nil: before=%d after=%d", want, got)
	}
}

func TestPS2024EquivalenceRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(0x2024))
	for range 300 {
		n := rng.Intn(2048)
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		ps2024Check(t, string(b))
	}
}

// TestPS2024AfterAllocs pins the perf premise. The string-direction
// After is allocation-free on every input. The mirror After is
// allocation-free on ASCII-dominated input (RuneCount's internal tail
// copy fits the 32-byte stack temporary) and never allocates more than
// its Before on any input — on current gc, RuneCount's non-ASCII slow
// path heap-copies the tail from the first non-ASCII byte, which is
// strictly no larger than the Before's full copy.
func TestPS2024AfterAllocs(t *testing.T) {
	mixed := strings.Repeat("héllo wörld … ", 64)  // well past any stack-temporary budget
	ascii := strings.Repeat("abcdefgh", 143) + "é" // non-ASCII only in the final short tail
	for _, s := range []string{mixed, ascii} {
		if avg := testing.AllocsPerRun(100, func() { sinkPS2024 = ps2024After(s) }); avg != 0 {
			t.Fatalf("utf8.RuneCountInString(%.16q...) allocates: %v allocs/op", s, avg)
		}
	}
	asciiB := []byte(ascii)
	if avg := testing.AllocsPerRun(100, func() { sinkPS2024 = ps2024AfterMirror(asciiB) }); avg != 0 {
		t.Fatalf("utf8.RuneCount(ascii bytes) allocates: %v allocs/op", avg)
	}
	mixedB := []byte(mixed)
	before := testing.AllocsPerRun(100, func() { sinkPS2024 = ps2024BeforeMirror(mixedB) })
	after := testing.AllocsPerRun(100, func() { sinkPS2024 = ps2024AfterMirror(mixedB) })
	if after > before {
		t.Fatalf("mirror After allocates more than Before on mixed input: %v > %v allocs/op", after, before)
	}
}

var sinkPS2024 int
