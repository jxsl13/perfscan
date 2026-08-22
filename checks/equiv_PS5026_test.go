package checks

// Runtime differential for PS5026: bytes.ContainsRune(b, r) vs the
// bytes.IndexByte(b, byte(r)) >= 0 form the fix emits — for every
// constant ASCII rune. The fix's safety argument is that ContainsRune is
// defined as IndexRune(b, r) >= 0, and IndexRune, for 0 <= r <
// utf8.RuneSelf, immediately delegates to IndexByte(b, byte(r)) — a pure
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
//   - targeted seeds: nil and empty haystacks, needle only at the start,
//     only at the end, repeated needles, NUL needles inside invalid
//     UTF-8, ASCII needles adjacent to and between the bytes of split
//     multi-byte runes, and long haystacks that cross the SIMD-scan
//     block boundaries;
//   - randomized long haystacks over the full byte range with a fixed
//     seed, probing present and often-absent ASCII runes and both ends.
//
// It also pins WHY the [0, 0x80) bound is load-bearing with concrete
// divergence witnesses (a multi-byte rune searches for its UTF-8
// encoding, utf8.RuneError searches for the first invalid-UTF-8 position,
// and an invalid/negative rune is a constant false — none of them is the
// truncated-byte scan), and the perf premise that NO side allocates.

import (
	"bytes"
	"math/rand"
	"testing"
	"unicode/utf8"
)

// ps5026Before* are the exact Before-shapes of the check: a constant
// ASCII rune through ContainsRune, plus the direct !ContainsRune form the
// fix absorbs.
func ps5026BeforeContains(b []byte, r rune) bool { return bytes.ContainsRune(b, r) }
func ps5026BeforeNot(b []byte, r rune) bool      { return !bytes.ContainsRune(b, r) }

// ps5026After* are the exact After-shapes of the check: IndexByte with
// the needle as a byte — under >= 0 for the plain form, as < 0 for the
// absorbed negation, and parenthesized inside a comparison operand. (The
// fix passes the untyped literal verbatim; byte(r) here is the same
// value, spelled explicitly because r arrives through a rune parameter.)
func ps5026AfterContains(b []byte, r rune) bool { return bytes.IndexByte(b, byte(r)) >= 0 }
func ps5026AfterNot(b []byte, r rune) bool      { return bytes.IndexByte(b, byte(r)) < 0 }
func ps5026AfterParen(b []byte, r rune, x bool) bool {
	return x == (bytes.IndexByte(b, byte(r)) >= 0)
}

func ps5026Check(t *testing.T, b []byte, r rune) {
	t.Helper()
	if r < 0 || r >= 0x80 {
		t.Fatalf("harness bug: needle rune %#x is not ASCII — outside the check's scope", r)
	}
	want := ps5026AfterContains(b, r)
	if got := ps5026BeforeContains(b, r); got != want {
		t.Fatalf("bytes.ContainsRune divergence on (%q, %q): before=%v after=%v", b, r, got, want)
	}
	// The absorbed negation: !(x >= 0) == (x < 0) for every int.
	if got := ps5026BeforeNot(b, r); got != !want {
		t.Fatalf("!bytes.ContainsRune divergence on (%q, %q): before=%v after=%v", b, r, got, !want)
	}
	if got := ps5026AfterNot(b, r); got != !want {
		t.Fatalf("bytes.IndexByte < 0 spelling divergence on (%q, %q): got %v want %v", b, r, got, !want)
	}
	// The parenthesized comparison-operand spelling is the same value in
	// both truth contexts.
	for _, x := range []bool{false, true} {
		if got, wantX := ps5026AfterParen(b, r, x), x == want; got != wantX {
			t.Fatalf("parenthesized operand divergence on (%q, %q, %v): got %v want %v", b, r, x, got, wantX)
		}
	}
}

// ps5026Alphabet stresses every comparison hazard: ASCII, NUL, the bytes
// of the two-byte U+00E9 (C3 A9) and three-byte U+3042 (E3 81 82) so
// truncated and complete multi-byte sequences arise at every alignment
// (exercising the RuneError-adjacent paths the ASCII bound steers clear
// of), a bare continuation byte, and 0xFF (never valid UTF-8).
var ps5026Alphabet = []byte{'a', 'z', 0x00, 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF}

func TestEquiv_PS5026_ExhaustiveShort(t *testing.T) {
	var enumerate func(buf []byte, depth int, emit func([]byte))
	enumerate = func(buf []byte, depth int, emit func([]byte)) {
		if depth == len(buf) {
			emit(append([]byte(nil), buf...))
			return
		}
		for _, c := range ps5026Alphabet {
			buf[depth] = c
			enumerate(buf, depth+1, emit)
		}
	}
	var haystacks [][]byte
	for n := 0; n <= 3; n++ {
		enumerate(make([]byte, n), 0, func(b []byte) { haystacks = append(haystacks, b) })
	}
	for _, b := range haystacks {
		// ALL 128 ASCII needle runes (the check's exact scope): absent,
		// present once, present repeatedly, at either boundary — every
		// combination arises from the cross product.
		for r := rune(0); r < 0x80; r++ {
			ps5026Check(t, b, r)
		}
	}
}

func TestEquiv_PS5026_TargetedSeeds(t *testing.T) {
	long := bytes.Repeat([]byte("payload="), 4096)
	seeds := [][]byte{
		nil, // nil haystack — []byte's extra edge over strings
		{},
		[]byte("z"),
		[]byte("\xff"), // one-byte NON-UTF-8 haystack
		[]byte("za"), []byte("az"), []byte("zz"),
		[]byte("zaz"), []byte("aza"),
		[]byte("only one z here"),
		[]byte("z at both ends and in the z middle z"),
		[]byte("no needle at all"),
		[]byte("\x00\x00a\x00"),                                    // NUL runs
		[]byte("\xff\xfe\xfd"),                                     // invalid UTF-8 throughout
		[]byte("\xc3\xa9\xc3\xa9z"),                                // needle after complete multi-byte runes
		[]byte("\xc3z\xa9"),                                        // needle between the bytes of a split rune
		[]byte("éz…z€"),                                            // multi-byte runes around the needle
		bytes.Repeat([]byte("z"), 1000),                            // needle everywhere
		long,                                                       // long, needle absent
		append(long[:len(long):len(long)], 'z'),                    // long, needle only at the very end
		append([]byte("z"), long...),                               // long, needle only at the very start
		append(append(append([]byte(nil), long...), 'z'), long...), // long, needle in the middle
	}
	probes := []rune{'z', 'a', '=', 0x00, 0x7F, '/'}
	for _, b := range seeds {
		for _, r := range probes {
			ps5026Check(t, b, r)
		}
	}
}

func TestEquiv_PS5026_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25026026))
	randBytes := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return b
	}
	for range 20000 {
		b := randBytes(rng.Intn(257))
		// A random ASCII rune (sometimes present), a rune from a narrow
		// range (often absent), and — when b starts/ends ASCII — its
		// boundary bytes (guaranteed hits at the boundaries).
		ps5026Check(t, b, rune(rng.Intn(0x80)))
		ps5026Check(t, b, rune(rng.Intn(4)))
		if len(b) > 0 && b[0] < 0x80 {
			ps5026Check(t, b, rune(b[0]))
		}
		if len(b) > 0 && b[len(b)-1] < 0x80 {
			ps5026Check(t, b, rune(b[len(b)-1]))
		}
	}
}

// TestEquiv_PS5026_NonASCIIDivergenceWitnesses pins WHY the check's
// [0, 0x80) needle bound is load-bearing (and why non-ASCII constant
// runes are excluded entirely, not even advisory): outside that range
// ContainsRune and the truncated-byte IndexByte answer DIFFERENT
// questions. If the stdlib ever changed any of these branches, this test
// — not a linter user — finds out.
func TestEquiv_PS5026_NonASCIIDivergenceWitnesses(t *testing.T) {
	// A multi-byte rune searches for its UTF-8 encoding: on the haystack
	// consisting of the single raw byte 0xE9, ContainsRune(b, 'é') is
	// false (no "\xc3\xa9" present) while IndexByte(b, byte('é')) ==
	// IndexByte(b, 0xE9) finds the byte at position 0.
	b := []byte{0xE9}
	if bytes.ContainsRune(b, 'é') {
		t.Fatalf("bytes.ContainsRune(%q, 'é') = true, want false: the stdlib changed — re-audit PS5026's ASCII bound", b)
	}
	if bytes.IndexByte(b, byte('é')) != 0 {
		t.Fatalf("bytes.IndexByte(%q, %#02x) != 0: the divergence witness lost its teeth", b, byte('é'))
	}
	// utf8.RuneError searches for the first INVALID-UTF-8 position, not
	// the truncated byte 0xFD: on "\xff" ContainsRune is true while
	// IndexByte(b, 0xFD) finds nothing. (The truncations are computed
	// through runtime rune variables: the CONSTANT conversions
	// byte(0xFFFD) and byte(-1) do not even compile — one more reason no
	// verbatim fix exists outside the ASCII range.)
	bad := []byte("\xff")
	runeError := rune(utf8.RuneError)
	if !bytes.ContainsRune(bad, utf8.RuneError) {
		t.Fatalf("bytes.ContainsRune(%q, RuneError) = false, want true (invalid-UTF-8 search): the stdlib changed — re-audit PS5026's ASCII bound", bad)
	}
	if bytes.IndexByte(bad, byte(runeError)) != -1 {
		t.Fatalf("bytes.IndexByte(%q, %#02x) != -1: the divergence witness lost its teeth", bad, byte(runeError))
	}
	// An invalid (negative or > MaxRune) rune is a constant false, while
	// the truncated byte can perfectly well be present.
	neg := []byte{0xFF}
	minusOne := rune(-1)
	if bytes.ContainsRune(neg, -1) {
		t.Fatalf("bytes.ContainsRune(%q, -1) = true, want false (invalid rune): the stdlib changed — re-audit PS5026's bound", neg)
	}
	if bytes.IndexByte(neg, byte(minusOne)) != 0 {
		t.Fatalf("bytes.IndexByte(%q, 0xff) != 0: the divergence witness lost its teeth", neg)
	}
}

// TestEquiv_PS5026_BothSidesZeroAlloc pins the perf premise: neither side
// allocates (the needle is a constant rune, the haystack passes through
// untouched) — the entire win is instruction count: the ContainsRune and
// IndexRune wrapper frames and the rune-class dispatch.
func TestEquiv_PS5026_BothSidesZeroAlloc(t *testing.T) {
	b := []byte("service=checkout region=eu-west-1 shard=07 status=ok final=z")
	var sink bool
	allocs := testing.AllocsPerRun(100, func() {
		sink = bytes.ContainsRune(b, 'z')
		sink = sink != (bytes.IndexByte(b, 'z') >= 0)
		sink = sink != !bytes.ContainsRune(b, '=')
		sink = sink != (bytes.IndexByte(b, '=') < 0)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the ContainsRune/IndexByte quartet allocated %v times per run; PS5026's like-for-like premise no longer holds", allocs)
	}
}
