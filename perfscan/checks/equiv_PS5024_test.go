package checks

// Runtime differential for PS5024: strings.ContainsRune(s, r) vs the
// strings.IndexByte(s, byte(r)) >= 0 form the fix emits — for every
// constant ASCII rune. The fix's safety argument is that ContainsRune is
// defined as IndexRune(s, r) >= 0, and IndexRune, for 0 <= r <
// utf8.RuneSelf, immediately delegates to IndexByte(s, byte(r)) — a pure
// byte scan with no rune decoding, no UTF-8 validation, and no case
// folding — so the returned boolean is bit-identical for every haystack
// (with !(x >= 0) == (x < 0) covering the fix's absorbed negation). This
// suite pins that claim over:
//
//   - EXHAUSTIVE pairs: every haystack of length <= 3 over an adversarial
//     alphabet (ASCII, NUL, the bytes of multi-byte runes so truncated
//     and complete sequences arise at every alignment, a bare
//     continuation byte, and 0xFF) crossed with ALL 128 ASCII needle
//     runes — presence, absence, and boundary positions all arise from
//     the cross product;
//   - targeted seeds: empty haystacks, needle only at the start, only at
//     the end, repeated needles, NUL needles inside invalid UTF-8, ASCII
//     needles adjacent to and between the bytes of split multi-byte
//     runes, and long haystacks that cross the SIMD-scan block
//     boundaries;
//   - randomized long haystacks over the full byte range with a fixed
//     seed, probing present and often-absent ASCII runes and both ends.
//
// It also pins WHY the [0, 0x80) bound is load-bearing with concrete
// divergence witnesses (a multi-byte rune searches for its UTF-8
// encoding, utf8.RuneError searches for the first invalid-UTF-8 position,
// and an invalid/negative rune is a constant false — none of them is the
// truncated-byte scan), and the perf premise that NO side allocates.

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// ps5024Before* are the exact Before-shapes of the check: a constant
// ASCII rune through ContainsRune, plus the direct !ContainsRune form the
// fix absorbs.
func ps5024BeforeContains(s string, r rune) bool { return strings.ContainsRune(s, r) }
func ps5024BeforeNot(s string, r rune) bool      { return !strings.ContainsRune(s, r) }

// ps5024After* are the exact After-shapes of the check: IndexByte with
// the needle as a byte — under >= 0 for the plain form, as < 0 for the
// absorbed negation, and parenthesized inside a comparison operand. (The
// fix passes the untyped literal verbatim; byte(r) here is the same
// value, spelled explicitly because r arrives through a rune parameter.)
func ps5024AfterContains(s string, r rune) bool { return strings.IndexByte(s, byte(r)) >= 0 }
func ps5024AfterNot(s string, r rune) bool      { return strings.IndexByte(s, byte(r)) < 0 }
func ps5024AfterParen(s string, r rune, x bool) bool {
	return x == (strings.IndexByte(s, byte(r)) >= 0)
}

func ps5024Check(t *testing.T, s string, r rune) {
	t.Helper()
	if r < 0 || r >= 0x80 {
		t.Fatalf("harness bug: needle rune %#x is not ASCII — outside the check's scope", r)
	}
	want := ps5024AfterContains(s, r)
	if got := ps5024BeforeContains(s, r); got != want {
		t.Fatalf("strings.ContainsRune divergence on (%q, %q): before=%v after=%v", s, r, got, want)
	}
	// The absorbed negation: !(x >= 0) == (x < 0) for every int.
	if got := ps5024BeforeNot(s, r); got != !want {
		t.Fatalf("!strings.ContainsRune divergence on (%q, %q): before=%v after=%v", s, r, got, !want)
	}
	if got := ps5024AfterNot(s, r); got != !want {
		t.Fatalf("strings.IndexByte < 0 spelling divergence on (%q, %q): got %v want %v", s, r, got, !want)
	}
	// The parenthesized comparison-operand spelling is the same value in
	// both truth contexts.
	for _, x := range []bool{false, true} {
		if got, wantX := ps5024AfterParen(s, r, x), x == want; got != wantX {
			t.Fatalf("parenthesized operand divergence on (%q, %q, %v): got %v want %v", s, r, x, got, wantX)
		}
	}
}

// ps5024Alphabet stresses every comparison hazard: ASCII, NUL, the bytes
// of the two-byte U+00E9 (C3 A9) and three-byte U+3042 (E3 81 82) so
// truncated and complete multi-byte sequences arise at every alignment
// (exercising the RuneError-adjacent paths the ASCII bound steers clear
// of), a bare continuation byte, and 0xFF (never valid UTF-8).
var ps5024Alphabet = []byte{'a', 'z', 0x00, 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF}

func TestEquiv_PS5024_ExhaustiveShort(t *testing.T) {
	var enumerate func(buf []byte, depth int, emit func([]byte))
	enumerate = func(buf []byte, depth int, emit func([]byte)) {
		if depth == len(buf) {
			emit(append([]byte(nil), buf...))
			return
		}
		for _, c := range ps5024Alphabet {
			buf[depth] = c
			enumerate(buf, depth+1, emit)
		}
	}
	var haystacks []string
	for n := 0; n <= 3; n++ {
		enumerate(make([]byte, n), 0, func(b []byte) { haystacks = append(haystacks, string(b)) })
	}
	for _, s := range haystacks {
		// ALL 128 ASCII needle runes (the check's exact scope): absent,
		// present once, present repeatedly, at either boundary — every
		// combination arises from the cross product.
		for r := rune(0); r < 0x80; r++ {
			ps5024Check(t, s, r)
		}
	}
}

func TestEquiv_PS5024_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("payload=", 4096)
	seeds := []string{
		"",
		"z",
		"\xff", // one-byte NON-UTF-8 haystack
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
	probes := []rune{'z', 'a', '=', 0x00, 0x7F, '/'}
	for _, s := range seeds {
		for _, r := range probes {
			ps5024Check(t, s, r)
		}
	}
}

func TestEquiv_PS5024_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25024024))
	randString := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return string(b)
	}
	for range 20000 {
		s := randString(rng.Intn(257))
		// A random ASCII rune (sometimes present), a rune from a narrow
		// range (often absent), and — when s starts/ends ASCII — its
		// boundary bytes (guaranteed hits at the boundaries).
		ps5024Check(t, s, rune(rng.Intn(0x80)))
		ps5024Check(t, s, rune(rng.Intn(4)))
		if len(s) > 0 && s[0] < 0x80 {
			ps5024Check(t, s, rune(s[0]))
		}
		if len(s) > 0 && s[len(s)-1] < 0x80 {
			ps5024Check(t, s, rune(s[len(s)-1]))
		}
	}
}

// TestEquiv_PS5024_NonASCIIDivergenceWitnesses pins WHY the check's
// [0, 0x80) needle bound is load-bearing (and why non-ASCII constant
// runes are excluded entirely, not even advisory): outside that range
// ContainsRune and the truncated-byte IndexByte answer DIFFERENT
// questions. If the stdlib ever changed any of these branches, this test
// — not a linter user — finds out.
func TestEquiv_PS5024_NonASCIIDivergenceWitnesses(t *testing.T) {
	// A multi-byte rune searches for its UTF-8 encoding: on the haystack
	// consisting of the single raw byte 0xE9, ContainsRune(s, 'é') is
	// false (no "\xc3\xa9" present) while IndexByte(s, byte('é')) ==
	// IndexByte(s, 0xE9) finds the byte at position 0.
	s := string([]byte{0xE9})
	if strings.ContainsRune(s, 'é') {
		t.Fatalf("strings.ContainsRune(%q, 'é') = true, want false: the stdlib changed — re-audit PS5024's ASCII bound", s)
	}
	if strings.IndexByte(s, byte('é')) != 0 {
		t.Fatalf("strings.IndexByte(%q, %#02x) != 0: the divergence witness lost its teeth", s, byte('é'))
	}
	// utf8.RuneError searches for the first INVALID-UTF-8 position, not
	// the truncated byte 0xFD: on "\xff" ContainsRune is true while
	// IndexByte(s, 0xFD) finds nothing. (The truncations are computed
	// through runtime rune variables: the CONSTANT conversions
	// byte(0xFFFD) and byte(-1) do not even compile — one more reason no
	// verbatim fix exists outside the ASCII range.)
	bad := "\xff"
	runeError := rune(utf8.RuneError)
	if !strings.ContainsRune(bad, utf8.RuneError) {
		t.Fatalf("strings.ContainsRune(%q, RuneError) = false, want true (invalid-UTF-8 search): the stdlib changed — re-audit PS5024's ASCII bound", bad)
	}
	if strings.IndexByte(bad, byte(runeError)) != -1 {
		t.Fatalf("strings.IndexByte(%q, %#02x) != -1: the divergence witness lost its teeth", bad, byte(runeError))
	}
	// An invalid (negative or > MaxRune) rune is a constant false, while
	// the truncated byte can perfectly well be present.
	neg := string([]byte{0xFF})
	minusOne := rune(-1)
	if strings.ContainsRune(neg, -1) {
		t.Fatalf("strings.ContainsRune(%q, -1) = true, want false (invalid rune): the stdlib changed — re-audit PS5024's bound", neg)
	}
	if strings.IndexByte(neg, byte(minusOne)) != 0 {
		t.Fatalf("strings.IndexByte(%q, 0xff) != 0: the divergence witness lost its teeth", neg)
	}
}

// TestEquiv_PS5024_BothSidesZeroAlloc pins the perf premise: neither side
// allocates (the needle is a constant rune, the haystack passes through
// untouched) — the entire win is instruction count: the ContainsRune and
// IndexRune wrapper frames and the rune-class dispatch.
func TestEquiv_PS5024_BothSidesZeroAlloc(t *testing.T) {
	s := "service=checkout region=eu-west-1 shard=07 status=ok final=z"
	var sink bool
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.ContainsRune(s, 'z')
		sink = sink != (strings.IndexByte(s, 'z') >= 0)
		sink = sink != !strings.ContainsRune(s, '=')
		sink = sink != (strings.IndexByte(s, '=') < 0)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the ContainsRune/IndexByte quartet allocated %v times per run; PS5024's like-for-like premise no longer holds", allocs)
	}
}
