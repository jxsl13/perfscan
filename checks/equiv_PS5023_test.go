package checks

// Runtime differential for PS5023: strings.IndexRune(s, r) /
// bytes.IndexRune(b, r) vs the IndexByte forms the fix's rename produces —
// for every constant ASCII rune. The fix's safety argument is that for
// 0 <= r < utf8.RuneSelf, IndexRune's first switch case is literally
// "return IndexByte(s, byte(r))" — a pure byte scan with no rune
// decoding, no UTF-8 validation, and no case folding — so the returned
// index is bit-identical for every haystack. This suite pins that claim
// over:
//
//   - EXHAUSTIVE pairs: every haystack of length <= 3 over an adversarial
//     alphabet (ASCII, NUL, the bytes of multi-byte runes so truncated
//     and complete sequences arise at every alignment, a bare
//     continuation byte, and 0xFF) crossed with ALL 128 ASCII runes —
//     presence, absence, and boundary positions all arise from the cross
//     product, in both the string and the []byte world;
//   - targeted seeds: nil and empty haystacks, needle only at the start,
//     only at the end, repeated needles, NUL needles inside invalid
//     UTF-8, ASCII needles adjacent to and between the bytes of split
//     multi-byte runes, and long haystacks that cross the SIMD-scan
//     block boundaries;
//   - randomized long haystacks over the full byte range with a fixed
//     seed, probing present and often-absent ASCII runes and both ends.
//
// It also pins WHY each part of the gate is load-bearing with concrete
// divergence witnesses (a non-ASCII rune whose byte() truncation IS
// present in the haystack, utf8.RuneError over invalid UTF-8, a negative
// rune, and a beyond-range rune — the shapes a non-constant variable
// could smuggle in at runtime), and the perf premise that NO side
// allocates.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// ps5023Before* are the exact Before-shapes of the check: a constant
// ASCII rune through IndexRune in both packages.
func ps5023BeforeStr(s string, r rune) int { return strings.IndexRune(s, r) }
func ps5023BeforeByt(b []byte, r rune) int { return bytes.IndexRune(b, r) }

// ps5023After* are the exact After-shapes of the check: IndexByte with
// the same operand. At a fixed call site the operand is an untyped
// constant that the compiler converts to byte exactly as byte(r) does
// here — for r in [0, 0x80) the conversion is value-preserving, never
// truncating.
func ps5023AfterStr(s string, r rune) int { return strings.IndexByte(s, byte(r)) }
func ps5023AfterByt(b []byte, r rune) int { return bytes.IndexByte(b, byte(r)) }

func ps5023Check(t *testing.T, b []byte, r rune) {
	t.Helper()
	if r < 0 || r >= utf8.RuneSelf {
		t.Fatalf("harness bug: rune %#x is not ASCII — outside the check's scope", r)
	}
	s := string(b)

	want := ps5023AfterStr(s, r)
	if got := ps5023BeforeStr(s, r); got != want {
		t.Fatalf("strings.IndexRune divergence on (%q, %q): before=%d after=%d", s, r, got, want)
	}
	if got := ps5023AfterByt(b, r); got != want {
		t.Fatalf("bytes/strings IndexByte disagreement on (%q, %q): bytes=%d strings=%d", b, r, got, want)
	}
	if got := ps5023BeforeByt(b, r); got != want {
		t.Fatalf("bytes.IndexRune divergence on (%q, %q): before=%d after=%d", b, r, got, want)
	}
}

// ps5023Alphabet stresses every comparison hazard: ASCII, NUL, the bytes
// of the two-byte U+00E9 (C3 A9) and three-byte U+3042 (E3 81 82) so
// truncated and complete multi-byte sequences arise at every alignment
// (exercising the UTF-8-shaped haystacks the byte scan must treat as raw
// bytes), a bare continuation byte, and 0xFF (never valid UTF-8).
var ps5023Alphabet = []byte{'a', 'z', 0x00, 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF}

func TestEquiv_PS5023_ExhaustiveShort(t *testing.T) {
	var enumerate func(buf []byte, depth int, emit func([]byte))
	enumerate = func(buf []byte, depth int, emit func([]byte)) {
		if depth == len(buf) {
			emit(bytes.Clone(buf))
			return
		}
		for _, c := range ps5023Alphabet {
			buf[depth] = c
			enumerate(buf, depth+1, emit)
		}
	}
	var haystacks [][]byte
	for n := 0; n <= 3; n++ {
		enumerate(make([]byte, n), 0, func(b []byte) { haystacks = append(haystacks, b) })
	}
	for _, b := range haystacks {
		// ALL 128 ASCII runes (the check's exact scope): absent, present
		// once, present repeatedly, at either boundary — every combination
		// arises from the cross product.
		for r := rune(0); r < utf8.RuneSelf; r++ {
			ps5023Check(t, b, r)
		}
	}
}

func TestEquiv_PS5023_TargetedSeeds(t *testing.T) {
	long := bytes.Repeat([]byte("payload="), 4096)
	seeds := [][]byte{
		nil,
		{},
		[]byte("z"),
		[]byte("\xff"), // one-byte NON-UTF-8 haystack: a raw byte scan on both sides
		[]byte("za"), []byte("az"), []byte("zz"),
		[]byte("zaz"), []byte("aza"),
		[]byte("only one z here"),
		[]byte("z at both ends and in the z middle z"),
		[]byte("no needle at all"),
		[]byte("\x00\x00a\x00"),                         // NUL runs
		[]byte("\xff\xfe\xfd"),                          // invalid UTF-8 throughout
		[]byte("\xc3\xa9\xc3\xa9z"),                     // needle after complete multi-byte runes
		[]byte("\xc3z\xa9"),                             // needle between the bytes of a split rune
		[]byte("éz…z€"),                                 // multi-byte runes around the needle
		bytes.Repeat([]byte("z"), 1000),                 // needle everywhere
		long,                                            // long, needle absent
		append(bytes.Clone(long), 'z'),                  // long, needle only at the very end
		append([]byte("z"), long...),                    // long, needle only at the very start
		append(append(bytes.Clone(long), 'z'), long...), // long, needle in the middle
	}
	probes := []rune{'z', 'a', '=', 0x00, 0x7F, '/'}
	for _, b := range seeds {
		for _, r := range probes {
			ps5023Check(t, b, r)
		}
	}
}

func TestEquiv_PS5023_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25023023))
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
		ps5023Check(t, b, rune(rng.Intn(utf8.RuneSelf)))
		ps5023Check(t, b, rune(rng.Intn(4)))
		if len(b) > 0 && b[0] < utf8.RuneSelf {
			ps5023Check(t, b, rune(b[0]))
		}
		if len(b) > 0 && b[len(b)-1] < utf8.RuneSelf {
			ps5023Check(t, b, rune(b[len(b)-1]))
		}
	}
}

// TestEquiv_PS5023_DivergenceWitnesses pins WHY every part of the check's
// gate is load-bearing — each excluded operand shape has a concrete input
// where IndexRune and IndexByte(s, byte(r)) genuinely disagree, which is
// exactly what a NON-CONSTANT rune variable could smuggle in at runtime
// (and why non-constant operands are never matched, not even advisory).
// If the stdlib ever changed any of these behaviors, this test — not a
// linter user — finds out.
func TestEquiv_PS5023_DivergenceWitnesses(t *testing.T) {
	// A non-ASCII rune: IndexRune searches for the multi-byte UTF-8
	// encoding of U+00E9 (C3 A9); byte('é') TRUNCATES to 0xE9, which IS
	// present in the haystack as a raw byte. The haystack is built at
	// runtime: the invalid-UTF-8 CONSTANT would (correctly) trip
	// staticcheck, and feeding it raw bytes is exactly the point.
	nonUTF8 := string([]byte{0xE9, ' ', 'x'})
	eAcute := rune('é') // a VARIABLE: byte(eAcute) is the runtime truncation a non-constant operand would perform (the constant conversion byte('é') would not even compile)
	if got := strings.IndexRune(nonUTF8, eAcute); got != -1 {
		t.Fatalf("strings.IndexRune(%q, 'é') = %d, want -1 (multi-byte search): the stdlib changed — re-audit PS5023's ASCII bound", nonUTF8, got)
	}
	if got := strings.IndexByte(nonUTF8, byte(eAcute)); got != 0 {
		t.Fatalf("strings.IndexByte(%q, byte('é')) = %d, want 0", nonUTF8, got)
	}

	// utf8.RuneError does not search for a byte at all: it returns the
	// first INVALID-UTF-8 position. byte(0xFFFD) truncates to 0xFD.
	invalid := string([]byte{'a', 0xFF, 0xFD})
	runeErr := rune(utf8.RuneError) // a variable, for the same reason
	if got := strings.IndexRune(invalid, runeErr); got != 1 {
		t.Fatalf("strings.IndexRune(%q, RuneError) = %d, want 1 (first invalid position): the stdlib changed — re-audit PS5023", invalid, got)
	}
	if got := strings.IndexByte(invalid, byte(runeErr)); got != 2 {
		t.Fatalf("strings.IndexByte(%q, %#02x) = %d, want 2", invalid, byte(runeErr), got)
	}

	// A negative rune: IndexRune hits !utf8.ValidRune and is a constant
	// -1; byte(-1) of a runtime value is 0xFF, which IS present.
	neg := rune(-1)
	ff := string([]byte{0xFF})
	if got := strings.IndexRune(ff, neg); got != -1 {
		t.Fatalf("strings.IndexRune(%q, -1) = %d, want -1 (invalid rune): the stdlib changed — re-audit PS5023", ff, got)
	}
	if got := strings.IndexByte(ff, byte(neg)); got != 0 {
		t.Fatalf("strings.IndexByte(%q, byte(-1)) = %d, want 0", ff, got)
	}

	// Beyond the rune range: also the constant -1 path, while the
	// truncation 0x110000 & 0xFF == 0x00 IS present.
	big := rune(0x110000)
	nul := string([]byte{0x00})
	if got := strings.IndexRune(nul, big); got != -1 {
		t.Fatalf("strings.IndexRune(%q, 0x110000) = %d, want -1 (invalid rune): the stdlib changed — re-audit PS5023", nul, got)
	}
	if got := strings.IndexByte(nul, byte(big)); got != 0 {
		t.Fatalf("strings.IndexByte(%q, byte(0x110000)) = %d, want 0", nul, got)
	}

	// The bytes package mirrors every witness.
	if got := bytes.IndexRune([]byte(nonUTF8), eAcute); got != -1 {
		t.Fatalf("bytes.IndexRune(%q, 'é') = %d, want -1: the stdlib changed — re-audit PS5023", nonUTF8, got)
	}
	if got := bytes.IndexByte([]byte(nonUTF8), byte(eAcute)); got != 0 {
		t.Fatalf("bytes.IndexByte(%q, byte('é')) = %d, want 0", nonUTF8, got)
	}
	if got := bytes.IndexRune([]byte(invalid), runeErr); got != 1 {
		t.Fatalf("bytes.IndexRune(%q, RuneError) = %d, want 1: the stdlib changed — re-audit PS5023", invalid, got)
	}
	if got := bytes.IndexRune([]byte(ff), neg); got != -1 {
		t.Fatalf("bytes.IndexRune(%q, -1) = %d, want -1: the stdlib changed — re-audit PS5023", ff, got)
	}
}

// TestEquiv_PS5023_BothSidesZeroAlloc pins the perf premise: neither side
// allocates — the entire win is instruction count: the non-inlined
// IndexRune wrapper frame and its range-dispatch switch.
func TestEquiv_PS5023_BothSidesZeroAlloc(t *testing.T) {
	s := "service=checkout region=eu-west-1 shard=07 status=ok final=z"
	b := []byte(s)
	var sink int
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.IndexRune(s, 'z')
		sink += bytes.IndexRune(b, 'z')
		sink += strings.IndexByte(s, 'z')
		sink += bytes.IndexByte(b, 'z')
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the IndexRune/IndexByte quartet allocated %v times per run; PS5023's like-for-like premise no longer holds", allocs)
	}
}
