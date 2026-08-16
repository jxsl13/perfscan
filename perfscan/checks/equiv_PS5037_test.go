package checks

// Runtime differential for PS5037: every emptiness comparison of
// utf8.RuneCountInString(s) / utf8.RuneCount(b) against the literal 0
// (and the 1-boundary forms) vs the len(...) form the fix emits. The
// fix's safety argument: an empty input has 0 runes and len 0, and ANY
// non-empty input has at least one rune AND len >= 1 — the decode loop
// yields exactly one utf8.RuneError rune of width 1 per erroneous byte,
// so invalid UTF-8 can never make a non-empty input count 0 runes.
// Hence count(x) and len(x) are each 0 exactly when x is empty, and all
// six (operator, literal) emptiness pairs evaluate identically. This
// suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every string/[]byte of length <= 3 over
//     an adversarial alphabet (ASCII, NUL, a UTF-8 lead byte, a bare
//     continuation byte, and 0xFF — so truncated, overlong-looking and
//     misaligned sequences arise at every position), plus every single
//     byte value 0..255 alone;
//   - targeted seeds: the empty string, nil and empty (non-nil)
//     []byte, pure ASCII, multi-byte scripts, combining marks, a lone
//     utf8.RuneError, maximal 4-byte runes, NUL-bearing strings, and
//     classic invalid sequences (lone continuation, truncated 2/3/4
//     byte starts, 0xFE/0xFF, surrogate-range encodings, overlong
//     encodings);
//   - randomized long inputs (up to 4 KiB) over the full byte range
//     with a fixed seed — long enough to cross RuneCount's ASCII
//     fast-path chunking on every architecture.
//
// It also pins the perf premise: the len side never allocates (and
// neither do RuneCountInString or RuneCount's ASCII path; RuneCount's
// non-ASCII tail copy on current gc is a bonus the rewrite removes).

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// ps5037Before* are the exact Before-shapes of the check (all six
// comparison forms); ps5037After* are the exact After-shapes the fix
// emits. Every Before must agree with its After on EVERY input.
func ps5037BeforeSEQ(s string) bool { return utf8.RuneCountInString(s) == 0 }
func ps5037BeforeSNE(s string) bool { return utf8.RuneCountInString(s) != 0 }
func ps5037BeforeSGT(s string) bool { return utf8.RuneCountInString(s) > 0 }
func ps5037BeforeSLE(s string) bool { return utf8.RuneCountInString(s) <= 0 }
func ps5037BeforeSGE(s string) bool { return utf8.RuneCountInString(s) >= 1 }
func ps5037BeforeSLT(s string) bool { return utf8.RuneCountInString(s) < 1 }

func ps5037AfterSEQ(s string) bool { return len(s) == 0 }
func ps5037AfterSNE(s string) bool { return len(s) != 0 }
func ps5037AfterSGT(s string) bool { return len(s) > 0 }
func ps5037AfterSLE(s string) bool { return len(s) <= 0 }
func ps5037AfterSGE(s string) bool { return len(s) >= 1 }
func ps5037AfterSLT(s string) bool { return len(s) < 1 }

func ps5037BeforeBEQ(b []byte) bool { return utf8.RuneCount(b) == 0 }
func ps5037BeforeBNE(b []byte) bool { return utf8.RuneCount(b) != 0 }
func ps5037BeforeBGT(b []byte) bool { return utf8.RuneCount(b) > 0 }
func ps5037BeforeBLE(b []byte) bool { return utf8.RuneCount(b) <= 0 }
func ps5037BeforeBGE(b []byte) bool { return utf8.RuneCount(b) >= 1 }
func ps5037BeforeBLT(b []byte) bool { return utf8.RuneCount(b) < 1 }

func ps5037AfterBEQ(b []byte) bool { return len(b) == 0 }
func ps5037AfterBNE(b []byte) bool { return len(b) != 0 }
func ps5037AfterBGT(b []byte) bool { return len(b) > 0 }
func ps5037AfterBLE(b []byte) bool { return len(b) <= 0 }
func ps5037AfterBGE(b []byte) bool { return len(b) >= 1 }
func ps5037AfterBLT(b []byte) bool { return len(b) < 1 }

func ps5037Check(t *testing.T, s string) {
	t.Helper()
	pairs := []struct {
		op            string
		before, after bool
	}{
		{"== 0", ps5037BeforeSEQ(s), ps5037AfterSEQ(s)},
		{"!= 0", ps5037BeforeSNE(s), ps5037AfterSNE(s)},
		{"> 0", ps5037BeforeSGT(s), ps5037AfterSGT(s)},
		{"<= 0", ps5037BeforeSLE(s), ps5037AfterSLE(s)},
		{">= 1", ps5037BeforeSGE(s), ps5037AfterSGE(s)},
		{"< 1", ps5037BeforeSLT(s), ps5037AfterSLT(s)},
	}
	for _, p := range pairs {
		if p.before != p.after {
			t.Errorf("utf8.RuneCountInString(%q) %s = %v, len %s = %v", s, p.op, p.before, p.op, p.after)
		}
	}
	b := []byte(s)
	bpairs := []struct {
		op            string
		before, after bool
	}{
		{"== 0", ps5037BeforeBEQ(b), ps5037AfterBEQ(b)},
		{"!= 0", ps5037BeforeBNE(b), ps5037AfterBNE(b)},
		{"> 0", ps5037BeforeBGT(b), ps5037AfterBGT(b)},
		{"<= 0", ps5037BeforeBLE(b), ps5037AfterBLE(b)},
		{">= 1", ps5037BeforeBGE(b), ps5037AfterBGE(b)},
		{"< 1", ps5037BeforeBLT(b), ps5037AfterBLT(b)},
	}
	for _, p := range bpairs {
		if p.before != p.after {
			t.Errorf("utf8.RuneCount(%q) %s = %v, len %s = %v", b, p.op, p.before, p.op, p.after)
		}
	}
}

func TestPS5037Equivalence(t *testing.T) {
	// Targeted seeds: boundaries and every classic invalid-UTF-8 shape.
	seeds := []string{
		"",
		"a",
		"ab",
		"hello, world",
		"héllo",
		"日本語テキスト",
		"é́́",                      // combining marks
		string(utf8.RuneError),      // "�" itself
		"\U0010FFFF",                // maximal code point
		"\x00",                      // NUL
		"a\x00b",                    // embedded NUL
		"\x80",                      // lone continuation byte
		"\x80\x80\x80",              // several
		"\xc3",                      // truncated 2-byte start
		"\xc3(",                     // 2-byte start + ASCII
		"\xe2\x82",                  // truncated 3-byte start
		"\xf0\x90\x80",              // truncated 4-byte start
		"\xfe",                      // never valid
		"\xff",                      // never valid
		"\xed\xa0\x80",              // surrogate-range encoding
		"\xc0\xaf",                  // overlong encoding
		"\xf8\x88\x80\x80\x80",      // invalid 5-byte form
		strings.Repeat("x", 4096),   // long pure ASCII (fast-path chunks)
		strings.Repeat("é", 2048),   // long multi-byte
		strings.Repeat("\xff", 512), // long invalid
	}
	for _, s := range seeds {
		ps5037Check(t, s)
	}

	// The nil []byte and the empty non-nil []byte: 0 runes, len 0.
	for _, b := range [][]byte{nil, {}} {
		if utf8.RuneCount(b) != 0 || len(b) != 0 {
			t.Fatalf("nil/empty []byte: RuneCount=%d len=%d", utf8.RuneCount(b), len(b))
		}
		if ps5037BeforeBEQ(b) != ps5037AfterBEQ(b) || ps5037BeforeBGT(b) != ps5037AfterBGT(b) {
			t.Errorf("nil/empty []byte diverges")
		}
	}

	// Every single byte value alone — valid ASCII, lead bytes,
	// continuation bytes, 0xFE/0xFF: each is exactly one rune (possibly
	// RuneError) and len 1.
	for v := 0; v < 256; v++ {
		ps5037Check(t, string([]byte{byte(v)}))
	}

	// Exhaustive short inputs over an adversarial alphabet: ASCII, NUL,
	// a lead byte, a bare continuation byte, and 0xFF, so truncated and
	// misaligned sequences arise at every position.
	alphabet := []byte{'a', 0x00, 0xc3, 0x80, 0xff}
	var rec func(prefix []byte, depth int)
	rec = func(prefix []byte, depth int) {
		ps5037Check(t, string(prefix))
		if depth == 3 {
			return
		}
		for _, c := range alphabet {
			rec(append(prefix, c), depth+1)
		}
	}
	rec(nil, 0)

	// Randomized long inputs over the full byte range, fixed seed.
	rng := rand.New(rand.NewSource(5037))
	for i := 0; i < 200; i++ {
		n := rng.Intn(4096)
		buf := make([]byte, n)
		for j := range buf {
			buf[j] = byte(rng.Intn(256))
		}
		ps5037Check(t, string(buf))
	}
}

// TestPS5037NoAllocs pins the perf premise: the len side NEVER
// allocates, and neither does RuneCountInString or RuneCount's ASCII
// path. RuneCount's non-ASCII path DOES allocate on current gc — it
// re-materializes string(p[n:]) from the first non-ASCII byte (the
// same artifact PS2024 documents) — which the rewrite removes as a
// bonus; that behavior is toolchain-dependent, so it is asserted only
// as an upper bound of the len side, not pinned exactly.
func TestPS5037NoAllocs(t *testing.T) {
	s := strings.Repeat("naïve — 日本語 £", 64)
	ascii := []byte(strings.Repeat("plain ascii line\n", 64))
	b := []byte(s)
	var sink bool
	if n := testing.AllocsPerRun(100, func() { sink = utf8.RuneCountInString(s) == 0 }); n != 0 {
		t.Errorf("utf8.RuneCountInString comparison allocates: %v allocs/op", n)
	}
	if n := testing.AllocsPerRun(100, func() { sink = utf8.RuneCount(ascii) > 0 }); n != 0 {
		t.Errorf("utf8.RuneCount ASCII comparison allocates: %v allocs/op", n)
	}
	if n := testing.AllocsPerRun(100, func() { sink = len(s) == 0 || len(b) > 0 || len(ascii) > 0 }); n != 0 {
		t.Errorf("len comparison allocates: %v allocs/op", n)
	}
	_ = sink
}
