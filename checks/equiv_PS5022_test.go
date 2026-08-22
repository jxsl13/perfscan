package checks

// Runtime differential for PS5022: strings.IndexAny(s, cut) /
// strings.ContainsAny(s, cut) / bytes.IndexAny(b, cut) /
// bytes.ContainsAny(b, cut) vs the IndexByte forms the fix emits — for
// every ONE-ASCII-BYTE cutset. The fix's safety argument is that for
// len(chars) == 1 IndexAny sets r = rune(chars[0]) and, since r <
// utf8.RuneSelf for an ASCII byte, delegates to IndexRune(s, r), which
// for r < RuneSelf delegates to IndexByte(s, byte(r)) — a pure byte scan
// with no rune decoding, no UTF-8 validation, and no case folding — so
// the returned index is bit-identical for every haystack, and
// ContainsAny(s, chars), defined as IndexAny(s, chars) >= 0, inherits the
// identity verbatim (with !(x >= 0) == (x < 0) covering the fix's
// absorbed negation). bytes.IndexAny's extra len(s) == 1 fast path agrees
// byte-for-byte too: an ASCII probe into a one-byte haystack is the same
// byte comparison on both sides. This suite pins that claim over:
//
//   - EXHAUSTIVE pairs: every haystack of length <= 3 over an adversarial
//     alphabet (ASCII, NUL, the bytes of multi-byte runes so truncated
//     and complete sequences arise at every alignment, a bare
//     continuation byte, and 0xFF) crossed with ALL 128 ASCII cutset
//     bytes — presence, absence, and boundary positions all arise from
//     the cross product, in both the string and the []byte world;
//   - targeted seeds: nil and empty haystacks, needle only at the start,
//     only at the end, repeated needles, NUL needles inside invalid
//     UTF-8, ASCII needles adjacent to and between the bytes of split
//     multi-byte runes, and long haystacks that cross the SIMD-scan
//     block boundaries;
//   - randomized long haystacks over the full byte range with a fixed
//     seed, probing present and often-absent ASCII bytes and both ends.
//
// It also pins WHY the < 0x80 bound is load-bearing with a concrete
// divergence witness (a single-byte cutset >= 0x80 makes IndexAny search
// for the first invalid-UTF-8 position, not the byte), and the perf
// premise that NO side allocates.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps5022Before* are the exact Before-shapes of the check: a one-ASCII-byte
// cutset through IndexAny/ContainsAny in both packages, plus the direct
// !ContainsAny forms the fix absorbs.
func ps5022BeforeStrIndex(s, cut string) int        { return strings.IndexAny(s, cut) }
func ps5022BeforeStrContains(s, cut string) bool    { return strings.ContainsAny(s, cut) }
func ps5022BeforeStrNot(s, cut string) bool         { return !strings.ContainsAny(s, cut) }
func ps5022BeforeBytIndex(b []byte, cut string) int { return bytes.IndexAny(b, cut) }
func ps5022BeforeBytContains(b []byte, cut string) bool {
	return bytes.ContainsAny(b, cut)
}
func ps5022BeforeBytNot(b []byte, cut string) bool { return !bytes.ContainsAny(b, cut) }

// ps5022After* are the exact After-shapes of the check: IndexByte with the
// cutset literal indexed as cut[0] — as a drop-in call for the IndexAny
// form, under >= 0 for the ContainsAny form, as < 0 for the absorbed
// negation, and parenthesized inside a comparison operand.
func ps5022AfterStrIndex(s, cut string) int        { return strings.IndexByte(s, cut[0]) }
func ps5022AfterStrContains(s, cut string) bool    { return strings.IndexByte(s, cut[0]) >= 0 }
func ps5022AfterStrNot(s, cut string) bool         { return strings.IndexByte(s, cut[0]) < 0 }
func ps5022AfterBytIndex(b []byte, cut string) int { return bytes.IndexByte(b, cut[0]) }
func ps5022AfterBytContains(b []byte, cut string) bool {
	return bytes.IndexByte(b, cut[0]) >= 0
}
func ps5022AfterBytNot(b []byte, cut string) bool { return bytes.IndexByte(b, cut[0]) < 0 }
func ps5022AfterStrParen(s, cut string, x bool) bool {
	return x == (strings.IndexByte(s, cut[0]) >= 0)
}

func ps5022Check(t *testing.T, b []byte, c byte) {
	t.Helper()
	if c >= 0x80 {
		t.Fatalf("harness bug: cutset byte %#02x is not ASCII — outside the check's scope", c)
	}
	cut := string([]byte{c})
	s := string(b)

	wantIdx := ps5022AfterStrIndex(s, cut)
	if got := ps5022BeforeStrIndex(s, cut); got != wantIdx {
		t.Fatalf("strings.IndexAny divergence on (%q, %q): before=%d after=%d", s, cut, got, wantIdx)
	}
	if got := ps5022AfterBytIndex(b, cut); got != wantIdx {
		t.Fatalf("bytes/strings IndexByte disagreement on (%q, %q): bytes=%d strings=%d", b, cut, got, wantIdx)
	}
	if got := ps5022BeforeBytIndex(b, cut); got != wantIdx {
		t.Fatalf("bytes.IndexAny divergence on (%q, %q): before=%d after=%d", b, cut, got, wantIdx)
	}

	wantHas := wantIdx >= 0
	if got := ps5022BeforeStrContains(s, cut); got != wantHas {
		t.Fatalf("strings.ContainsAny divergence on (%q, %q): before=%v after=%v", s, cut, got, wantHas)
	}
	if got := ps5022AfterStrContains(s, cut); got != wantHas {
		t.Fatalf("strings.IndexByte >= 0 spelling divergence on (%q, %q): got %v want %v", s, cut, got, wantHas)
	}
	if got := ps5022BeforeBytContains(b, cut); got != wantHas {
		t.Fatalf("bytes.ContainsAny divergence on (%q, %q): before=%v after=%v", b, cut, got, wantHas)
	}
	if got := ps5022AfterBytContains(b, cut); got != wantHas {
		t.Fatalf("bytes.IndexByte >= 0 spelling divergence on (%q, %q): got %v want %v", b, cut, got, wantHas)
	}
	// The absorbed negation: !(x >= 0) == (x < 0) for every int.
	if got := ps5022BeforeStrNot(s, cut); got != !wantHas {
		t.Fatalf("!strings.ContainsAny divergence on (%q, %q): before=%v after=%v", s, cut, got, !wantHas)
	}
	if got := ps5022AfterStrNot(s, cut); got != !wantHas {
		t.Fatalf("strings.IndexByte < 0 spelling divergence on (%q, %q): got %v want %v", s, cut, got, !wantHas)
	}
	if got := ps5022BeforeBytNot(b, cut); got != !wantHas {
		t.Fatalf("!bytes.ContainsAny divergence on (%q, %q): before=%v after=%v", b, cut, got, !wantHas)
	}
	if got := ps5022AfterBytNot(b, cut); got != !wantHas {
		t.Fatalf("bytes.IndexByte < 0 spelling divergence on (%q, %q): got %v want %v", b, cut, got, !wantHas)
	}
	// The parenthesized comparison-operand spelling is the same value in
	// both truth contexts.
	for _, x := range []bool{false, true} {
		if got, want := ps5022AfterStrParen(s, cut, x), x == wantHas; got != want {
			t.Fatalf("parenthesized operand divergence on (%q, %q, %v): got %v want %v", s, cut, x, got, want)
		}
	}
}

// ps5022Alphabet stresses every comparison hazard: ASCII, NUL, the bytes
// of the two-byte U+00E9 (C3 A9) and three-byte U+3042 (E3 81 82) so
// truncated and complete multi-byte sequences arise at every alignment
// (exercising the RuneError-adjacent paths the ASCII bound steers clear
// of), a bare continuation byte, and 0xFF (never valid UTF-8).
var ps5022Alphabet = []byte{'a', 'z', 0x00, 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF}

func TestEquiv_PS5022_ExhaustiveShort(t *testing.T) {
	var enumerate func(buf []byte, depth int, emit func([]byte))
	enumerate = func(buf []byte, depth int, emit func([]byte)) {
		if depth == len(buf) {
			emit(bytes.Clone(buf))
			return
		}
		for _, c := range ps5022Alphabet {
			buf[depth] = c
			enumerate(buf, depth+1, emit)
		}
	}
	var haystacks [][]byte
	for n := 0; n <= 3; n++ {
		enumerate(make([]byte, n), 0, func(b []byte) { haystacks = append(haystacks, b) })
	}
	for _, b := range haystacks {
		// ALL 128 ASCII cutset bytes (the check's exact scope): absent,
		// present once, present repeatedly, at either boundary — every
		// combination arises from the cross product. The one-byte
		// haystacks also pin bytes.IndexAny's len(s) == 1 fast path,
		// including its s[0] >= 0x80 branch against an ASCII cutset.
		for c := 0; c < 0x80; c++ {
			ps5022Check(t, b, byte(c))
		}
	}
}

func TestEquiv_PS5022_TargetedSeeds(t *testing.T) {
	long := bytes.Repeat([]byte("payload="), 4096)
	seeds := [][]byte{
		nil,
		{},
		[]byte("z"),
		[]byte("\xff"), // one-byte NON-UTF-8 haystack: bytes.IndexAny's len(s)==1 RuneError branch
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
	probes := []byte{'z', 'a', '=', 0x00, 0x7F, '/'}
	for _, b := range seeds {
		for _, c := range probes {
			ps5022Check(t, b, c)
		}
	}
}

func TestEquiv_PS5022_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25022022))
	randBytes := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return b
	}
	for range 20000 {
		b := randBytes(rng.Intn(257))
		// A random ASCII byte (sometimes present), a byte from a narrow
		// range (often absent), and — when b starts/ends ASCII — its
		// boundary bytes (guaranteed hits at the boundaries).
		ps5022Check(t, b, byte(rng.Intn(0x80)))
		ps5022Check(t, b, byte(rng.Intn(4)))
		if len(b) > 0 && b[0] < 0x80 {
			ps5022Check(t, b, b[0])
		}
		if len(b) > 0 && b[len(b)-1] < 0x80 {
			ps5022Check(t, b, b[len(b)-1])
		}
	}
}

// TestEquiv_PS5022_NonASCIIDivergenceWitness pins WHY the check's < 0x80
// cutset bound is load-bearing (and why single non-ASCII-byte cutsets are
// excluded entirely, not even advisory): for a one-byte cutset >= 0x80,
// IndexAny remaps the cutset rune to utf8.RuneError and returns the first
// INVALID-UTF-8 position — NOT the first occurrence of the byte. On the
// witness "\xc3\x28" (an invalid two-byte lead) with cutset "\xff",
// IndexAny finds position 0 while IndexByte finds nothing. If the stdlib
// ever changed this behavior, this test — not a linter user — finds out.
func TestEquiv_PS5022_NonASCIIDivergenceWitness(t *testing.T) {
	// Both built at runtime: the invalid-UTF-8 CONSTANTS "\xc3\x28" and
	// "\xff" would (correctly) trip staticcheck's SA1011 — feeding
	// IndexAny invalid UTF-8 is exactly the point of this witness.
	s := string([]byte{0xc3, 0x28})
	cut := string([]byte{0xff})
	if got := strings.IndexAny(s, cut); got != 0 {
		t.Fatalf("strings.IndexAny(%q, %q) = %d, want 0 (RuneError remap): the stdlib changed — re-audit PS5022's ASCII bound", s, cut, got)
	}
	if got := strings.IndexByte(s, cut[0]); got != -1 {
		t.Fatalf("strings.IndexByte(%q, %#02x) = %d, want -1", s, cut[0], got)
	}
	if got := bytes.IndexAny([]byte(s), cut); got != 0 {
		t.Fatalf("bytes.IndexAny(%q, %q) = %d, want 0 (RuneError remap): the stdlib changed — re-audit PS5022's ASCII bound", s, cut, got)
	}
	if got := bytes.IndexByte([]byte(s), cut[0]); got != -1 {
		t.Fatalf("bytes.IndexByte(%q, %#02x) = %d, want -1", s, cut[0], got)
	}
}

// TestEquiv_PS5022_BothSidesZeroAlloc pins the perf premise: neither side
// allocates (the cutset is a string constant, never a constructed slice) —
// the entire win is instruction count: the cutset-length dispatch, the
// rune conversion and RuneSelf checks, and the IndexRune call frame.
func TestEquiv_PS5022_BothSidesZeroAlloc(t *testing.T) {
	s := "service=checkout region=eu-west-1 shard=07 status=ok final=z"
	b := []byte(s)
	var sinkI int
	var sinkB bool
	allocs := testing.AllocsPerRun(100, func() {
		sinkI = strings.IndexAny(s, "z")
		sinkI += bytes.IndexAny(b, "z")
		sinkI += strings.IndexByte(s, "z"[0])
		sinkI += bytes.IndexByte(b, "z"[0])
		sinkB = strings.ContainsAny(s, "=")
		sinkB = sinkB != bytes.ContainsAny(b, "=")
		sinkB = sinkB != (strings.IndexByte(s, "="[0]) >= 0)
		sinkB = sinkB != (bytes.IndexByte(b, "="[0]) < 0)
	})
	_, _ = sinkI, sinkB
	if allocs != 0 {
		t.Fatalf("the IndexAny/IndexByte octet allocated %v times per run; PS5022's like-for-like premise no longer holds", allocs)
	}
}
