package checks

// Runtime differential for PS5016: strings.Contains(s, sub) vs
// strings.IndexByte(s, sub[0]) >= 0 — and the negated pair !Contains vs
// IndexByte < 0 the fix's !-absorption emits — for every ONE-BYTE needle.
// The fix's safety argument is that strings.Contains(s, substr) is
// defined as strings.Index(s, substr) >= 0, and for len(substr) == 1
// strings.Index is specified (and implemented) as IndexByte(s, substr[0]):
// both paths perform the identical raw byte-wise search with no rune
// decoding, no UTF-8 validation, and no case folding anywhere, so the
// returned bool is equal for every haystack — and !(x >= 0) == (x < 0)
// for every int, covering the absorbed negation. The sub[0] wrap the fix
// emits is exercised alongside a plain byte spelling. This suite pins
// that claim over:
//
//   - EXHAUSTIVE pairs: every haystack of length <= 3 over an adversarial
//     alphabet (ASCII, NUL, the bytes of multi-byte runes so truncated
//     and complete sequences arise at every alignment, a bare
//     continuation byte, and 0xFF) crossed with ALL 256 possible needle
//     bytes — presence, absence, and boundary positions all arise from
//     the cross product;
//   - targeted seeds: the empty haystack, needle only at the start, only
//     at the end, repeated needles, NUL and 0xFF needles inside invalid
//     UTF-8, and long haystacks that cross the SIMD-scan block
//     boundaries;
//   - randomized long haystacks over the full byte range with a fixed
//     seed, probing a present byte, an often-absent byte, and both ends.
//
// It also pins the perf premise that NO side allocates — both needles are
// string constants, no slice is ever constructed — so the entire win is
// instruction count, and the comparison stays like-for-like.

import (
	"math/rand"
	"strings"
	"testing"
)

// ps5016Before* are the exact Before-shapes of the check: a one-byte
// constant needle through Contains, plus the direct !Contains form the
// fix absorbs.
func ps5016Before(s, sub string) bool    { return strings.Contains(s, sub) }
func ps5016BeforeNot(s, sub string) bool { return !strings.Contains(s, sub) }

// ps5016After* are the exact After-shapes of the check: the fix's sub[0]
// wrap under >= 0, a plain byte spelling as a cross-check, the absorbed
// negation as < 0, and the parenthesized spelling the fix emits inside
// comparison operands.
func ps5016After(s string, c byte) bool    { return strings.IndexByte(s, c) >= 0 }
func ps5016AfterLit(s, sub string) bool    { return strings.IndexByte(s, sub[0]) >= 0 }
func ps5016AfterNot(s string, c byte) bool { return strings.IndexByte(s, c) < 0 }
func ps5016AfterNotLit(s, sub string) bool { return strings.IndexByte(s, sub[0]) < 0 }
func ps5016AfterParen(s string, c byte, x bool) bool {
	return x == (strings.IndexByte(s, c) >= 0)
}

func ps5016Check(t *testing.T, s string, c byte) {
	t.Helper()
	// string([]byte{c}), NOT string(c): the integer conversion would
	// UTF-8-encode c and yield a TWO-byte needle for c >= 0x80. The
	// check's needle is a constant of decoded byte-length exactly 1, so
	// its runtime counterpart is the raw one-byte string.
	sub := string([]byte{c})
	if len(sub) != 1 {
		t.Fatalf("harness bug: needle %q is %d bytes, want 1", sub, len(sub))
	}
	after := ps5016After(s, c)
	if lit := ps5016AfterLit(s, sub); lit != after {
		t.Fatalf("IndexByte sub[0] spelling divergence on (%q, %q): lit=%v after=%v", s, sub, lit, after)
	}
	if before := ps5016Before(s, sub); before != after {
		t.Fatalf("Contains divergence on (%q, %q): before=%v after=%v", s, sub, before, after)
	}
	notAfter := ps5016AfterNot(s, c)
	if notAfter != !after {
		t.Fatalf("negation identity broken on (%q, %#02x): < 0 gave %v, want %v", s, c, notAfter, !after)
	}
	if before := ps5016BeforeNot(s, sub); before != notAfter {
		t.Fatalf("!Contains divergence on (%q, %q): before=%v after=%v", s, sub, before, notAfter)
	}
	if lit := ps5016AfterNotLit(s, sub); lit != notAfter {
		t.Fatalf("IndexByte sub[0] < 0 spelling divergence on (%q, %q): lit=%v after=%v", s, sub, lit, notAfter)
	}
	// The parenthesized comparison-operand spelling is the same value in
	// both truth contexts.
	for _, x := range []bool{false, true} {
		if got, want := ps5016AfterParen(s, c, x), x == ps5016Before(s, sub); got != want {
			t.Fatalf("parenthesized operand divergence on (%q, %#02x, %v): got %v want %v", s, c, x, got, want)
		}
	}
}

// ps5016Alphabet stresses every comparison hazard: ASCII, NUL, the bytes
// of the two-byte U+00E9 (C3 A9) and three-byte U+3042 (E3 81 82) so
// truncated and complete multi-byte sequences arise at every alignment,
// a bare continuation byte, and 0xFF (never valid UTF-8).
var ps5016Alphabet = []byte{'a', 'z', 0x00, 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF}

func TestEquiv_PS5016_ExhaustiveShort(t *testing.T) {
	var enumerate func(buf []byte, depth int, emit func(string))
	enumerate = func(buf []byte, depth int, emit func(string)) {
		if depth == len(buf) {
			emit(string(buf))
			return
		}
		for _, c := range ps5016Alphabet {
			buf[depth] = c
			enumerate(buf, depth+1, emit)
		}
	}
	var haystacks []string
	for n := 0; n <= 3; n++ {
		enumerate(make([]byte, n), 0, func(s string) { haystacks = append(haystacks, s) })
	}
	for _, s := range haystacks {
		// ALL 256 needle bytes: absent, present once, present repeatedly,
		// at either boundary — every combination arises from the cross
		// product.
		for c := 0; c < 256; c++ {
			ps5016Check(t, s, byte(c))
		}
	}
}

func TestEquiv_PS5016_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("payload=", 4096)
	seeds := []string{
		"",
		"z",
		"za", "az", "zz",
		"zaz", "aza",
		"only one z here",
		"z at both ends and in the z middle z",
		"no needle at all",
		"\x00\x00a\x00",           // NUL runs
		"\xff\xfe\xfd",            // invalid UTF-8 throughout
		"\xc3\xa9\xc3\xa9z",       // needle after complete multi-byte runes
		"\xc3z\xa9",               // needle between the bytes of a split rune
		"éz…z€",                   // multi-byte runes around the needle
		strings.Repeat("z", 1000), // needle everywhere
		long,                      // long, needle absent
		long + "z",                // long, needle only at the very end
		"z" + long,                // long, needle only at the very start
		long + "z" + long,         // long, needle in the middle
	}
	probes := []byte{'z', 'a', '=', 0x00, 0xC3, 0xA9, 0xFF}
	for _, s := range seeds {
		for _, c := range probes {
			ps5016Check(t, s, c)
		}
	}
}

func TestEquiv_PS5016_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25016016))
	randString := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return string(b)
	}
	for range 50000 {
		s := randString(rng.Intn(257))
		// A fully random byte (usually present), a byte from a narrow
		// range (often absent), and — when s is non-empty — its first
		// and last bytes (guaranteed hits at the boundaries).
		ps5016Check(t, s, byte(rng.Intn(256)))
		ps5016Check(t, s, byte(rng.Intn(4)))
		if len(s) > 0 {
			ps5016Check(t, s, s[0])
			ps5016Check(t, s, s[len(s)-1])
		}
	}
}

// TestEquiv_PS5016_BothSidesZeroAlloc pins the perf premise: neither the
// Before shape (a constant string needle — no slice is ever constructed)
// nor the After shapes allocate — the entire win is instruction count
// (the wrapper frames and the needle-length dispatch), not allocation
// behavior.
func TestEquiv_PS5016_BothSidesZeroAlloc(t *testing.T) {
	s := "service=checkout region=eu-west-1 shard=07 status=ok final=z"
	c := byte('z')
	var sink bool
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.Contains(s, "z")
		sink = sink != !strings.Contains(s, "z")
		sink = sink != (strings.IndexByte(s, c) >= 0)
		sink = sink != (strings.IndexByte(s, "z"[0]) >= 0)
		sink = sink != (strings.IndexByte(s, c) < 0)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the Contains/IndexByte quintet allocated %v times per run; PS5016's like-for-like premise no longer holds", allocs)
	}
}
