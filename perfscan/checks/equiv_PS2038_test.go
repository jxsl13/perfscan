package checks

// Runtime differential for PS2038: utf8.DecodeRuneInString(string(b)) vs
// utf8.DecodeRune(b), utf8.DecodeLastRuneInString(string(b)) vs
// utf8.DecodeLastRune(b), and the forward mirrors
// utf8.DecodeRune([]byte(s)) vs utf8.DecodeRuneInString(s) and
// utf8.DecodeLastRune([]byte(s)) vs utf8.DecodeLastRuneInString(s).
//
// The fix's safety argument is that unicode/utf8 implements each pair as
// ONE algorithm twice — the same inlineable ASCII fast path, one shared
// pair of slow paths over the same first/acceptRanges tables, and (for
// the Last pair) the identical UTFMax-bounded backward scan — with
// bodies differing only in whether the operand is indexed as []byte or
// string, while string(b) / []byte(s) copy bytes verbatim
// (string(nil []byte) == ""). So each pair returns the identical
// (r, size) on EVERY input, including empty ((RuneError, 0)), a lone
// continuation or invalid byte ((RuneError, 1)), and truncated multibyte
// sequences at either edge. This suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 4 over an
//     adversarial alphabet (NUL, plain ASCII, the last one-byte rune
//     0x7F, a bare continuation byte, the bytes of two- and three-byte
//     runes so truncated and complete sequences both arise at both
//     edges, a four-byte lead, the always-invalid overlong lead 0xC0,
//     and 0xFF);
//   - targeted seeds: empty, pure ASCII, mixed-width valid UTF-8, the
//     replacement char itself, an encoded surrogate half (invalid in
//     Go), overlong encodings, U+10FFFF, an out-of-range 0xF5 lead,
//     truncated sequences at the start and at the end of the input,
//     long runs of bare continuation bytes (the Last pair's backward
//     UTFMax guard), and long inputs;
//   - randomized long inputs over the full byte range with fixed seeds.
//
// It also pins the perf premise: all four After shapes are
// allocation-free on every input (they read the operand in place).

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// ps2038Before/After are the exact Before/After shapes of the check.
func ps2038Before(b []byte) (rune, int)            { return utf8.DecodeRuneInString(string(b)) }
func ps2038After(b []byte) (rune, int)             { return utf8.DecodeRune(b) }
func ps2038BeforeLast(b []byte) (rune, int)        { return utf8.DecodeLastRuneInString(string(b)) }
func ps2038AfterLast(b []byte) (rune, int)         { return utf8.DecodeLastRune(b) }
func ps2038BeforeForward(s string) (rune, int)     { return utf8.DecodeRune([]byte(s)) }
func ps2038AfterForward(s string) (rune, int)      { return utf8.DecodeRuneInString(s) }
func ps2038BeforeLastForward(s string) (rune, int) { return utf8.DecodeLastRune([]byte(s)) }
func ps2038AfterLastForward(s string) (rune, int)  { return utf8.DecodeLastRuneInString(s) }

// ps2038Check verifies all four pairs on one input's byte content.
func ps2038Check(t *testing.T, s string) {
	t.Helper()
	b := []byte(s)
	wr, ws := ps2038Before(b)
	gr, gs := ps2038After(b)
	if gr != wr || gs != ws {
		t.Fatalf("first-rune divergence on %q: DecodeRuneInString(string(b))=(%q,%d), DecodeRune(b)=(%q,%d)", s, wr, ws, gr, gs)
	}
	wr, ws = ps2038BeforeLast(b)
	gr, gs = ps2038AfterLast(b)
	if gr != wr || gs != ws {
		t.Fatalf("last-rune divergence on %q: DecodeLastRuneInString(string(b))=(%q,%d), DecodeLastRune(b)=(%q,%d)", s, wr, ws, gr, gs)
	}
	wr, ws = ps2038BeforeForward(s)
	gr, gs = ps2038AfterForward(s)
	if gr != wr || gs != ws {
		t.Fatalf("forward first-rune divergence on %q: DecodeRune([]byte(s))=(%q,%d), DecodeRuneInString(s)=(%q,%d)", s, wr, ws, gr, gs)
	}
	wr, ws = ps2038BeforeLastForward(s)
	gr, gs = ps2038AfterLastForward(s)
	if gr != wr || gs != ws {
		t.Fatalf("forward last-rune divergence on %q: DecodeLastRune([]byte(s))=(%q,%d), DecodeLastRuneInString(s)=(%q,%d)", s, wr, ws, gr, gs)
	}
}

func TestPS2038EquivalenceExhaustive(t *testing.T) {
	// Adversarial alphabet: NUL, ASCII, the 0x7F boundary, a bare
	// continuation byte, é (0xC3 0xA9), the bytes of € (0xE2 0x82 0xAC),
	// a four-byte lead (0xF0), the always-invalid overlong lead 0xC0,
	// and 0xFF. Truncated, complete, and illegally-recombined sequences
	// all arise at both edges from the cross product.
	alphabet := []byte{0x00, 'a', 0x7F, 0x80, 0xC3, 0xA9, 0xE2, 0x82, 0xAC, 0xF0, 0xC0, 0xFF}
	var rec func(prefix []byte, depth int)
	rec = func(prefix []byte, depth int) {
		ps2038Check(t, string(prefix))
		if depth == 0 {
			return
		}
		for _, c := range alphabet {
			rec(append(prefix, c), depth-1)
		}
	}
	rec(nil, 4)
}

func TestPS2038EquivalenceSeeds(t *testing.T) {
	seeds := []string{
		"",
		"plain ascii only",
		"héllo wörld … é€\U0001F4A9",
		"\U0001F4A9 leading four-byte rune",
		"�",                                  // the replacement char itself, validly encoded
		"\xed\xa0\x80",                       // encoded surrogate half — invalid in Go
		"\xc0\xaf",                           // overlong encoding of '/'
		"\xe0\x80\x80",                       // overlong three-byte encoding
		"\xf4\x8f\xbf\xbf",                   // U+10FFFF, the maximal rune
		"\xf5\x80\x80\x80",                   // lead beyond U+10FFFF
		"\xf0\x9f\x92",                       // truncated four-byte sequence at end
		"\x9f\x92\xa9",                       // truncated four-byte sequence at start
		"abc\xc3",                            // truncated two-byte sequence at end
		"\xc3abc",                            // orphaned two-byte lead at start
		"\x80\x80\x80\x80\x80\x80\x80\x80",   // long bare-continuation run (Last's UTFMax guard)
		"a\xffb\xfec",                        // 0xFF/0xFE interleaved with ASCII
		strings.Repeat("x", 4096),            // long pure ASCII
		strings.Repeat("héllo…", 1024),       // long mixed-width valid UTF-8
		strings.Repeat("\xc3x\xe2\x82", 512), // long invalid/truncated mix
	}
	for _, s := range seeds {
		ps2038Check(t, s)
	}
	// The reverse arms must also hold for a nil []byte: string(nil) == "".
	wr, ws := ps2038Before(nil)
	gr, gs := ps2038After(nil)
	if gr != wr || gs != ws {
		t.Fatalf("first-rune divergence on nil: before=(%q,%d) after=(%q,%d)", wr, ws, gr, gs)
	}
	wr, ws = ps2038BeforeLast(nil)
	gr, gs = ps2038AfterLast(nil)
	if gr != wr || gs != ws {
		t.Fatalf("last-rune divergence on nil: before=(%q,%d) after=(%q,%d)", wr, ws, gr, gs)
	}
	// And the canonical edge results themselves.
	if r, size := ps2038After(nil); r != utf8.RuneError || size != 0 {
		t.Fatalf("DecodeRune(nil) = (%q,%d), want (RuneError,0)", r, size)
	}
	if r, size := ps2038AfterLast(nil); r != utf8.RuneError || size != 0 {
		t.Fatalf("DecodeLastRune(nil) = (%q,%d), want (RuneError,0)", r, size)
	}
}

func TestPS2038EquivalenceRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(0x2038))
	for range 300 {
		n := rng.Intn(2048)
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		ps2038Check(t, string(b))
	}
}

// TestPS2038AfterAllocs pins the perf premise: all four After shapes
// read their operand in place and are allocation-free on every input —
// unlike the reverse Befores, whose string(b) conversion heap-copies
// the WHOLE slice (96 B and 1 alloc per call on the 85-byte operand of
// the check's MeasuredWin) to decode at most 4 bytes of it.
func TestPS2038AfterAllocs(t *testing.T) {
	mixed := strings.Repeat("héllo wörld … ", 8) // well past any stack-temporary budget
	b := []byte(mixed)
	if avg := testing.AllocsPerRun(100, func() { sinkPS2038r, sinkPS2038n = ps2038After(b) }); avg != 0 {
		t.Fatalf("utf8.DecodeRune(b) allocates: %v allocs/op", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sinkPS2038r, sinkPS2038n = ps2038AfterLast(b) }); avg != 0 {
		t.Fatalf("utf8.DecodeLastRune(b) allocates: %v allocs/op", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sinkPS2038r, sinkPS2038n = ps2038AfterForward(mixed) }); avg != 0 {
		t.Fatalf("utf8.DecodeRuneInString(s) allocates: %v allocs/op", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sinkPS2038r, sinkPS2038n = ps2038AfterLastForward(mixed) }); avg != 0 {
		t.Fatalf("utf8.DecodeLastRuneInString(s) allocates: %v allocs/op", avg)
	}
}

var (
	sinkPS2038r rune
	sinkPS2038n int
)
