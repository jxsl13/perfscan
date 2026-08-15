package checks

// Runtime differential for PS5014: bytes.Contains(b, []byte{c}) /
// bytes.Contains(b, []byte(sub)) vs bytes.IndexByte(b, c) >= 0 — and the
// negated pair !Contains vs IndexByte < 0 the fix's !-absorption emits —
// for every ONE-BYTE needle. The fix's safety argument is that
// bytes.Contains(b, sep) is defined as bytes.Index(b, sep) >= 0, and for
// len(sep) == 1 bytes.Index is implemented as IndexByte(s, sep[0]): both
// paths perform the identical raw byte-wise search with no rune decoding,
// no UTF-8 validation, and no case folding anywhere, so the returned bool
// is equal for every haystack — and !(x >= 0) == (x < 0) for every int,
// covering the absorbed negation. Both Before spellings the check
// rewrites are exercised (the composite literal []byte{c} and the
// conversion []byte(sub) with the sub[0] wrap the fix emits). This suite
// pins that claim over:
//
//   - EXHAUSTIVE pairs: every haystack of length <= 3 over an adversarial
//     alphabet (ASCII, NUL, the bytes of multi-byte runes so truncated
//     and complete sequences arise at every alignment, a bare
//     continuation byte, and 0xFF) crossed with ALL 256 possible needle
//     bytes — presence, absence, and boundary positions all arise from
//     the cross product;
//   - targeted seeds: nil and empty haystacks, needle only at the start,
//     only at the end, repeated needles, NUL and 0xFF needles inside
//     invalid UTF-8, and long haystacks that cross the SIMD-scan block
//     boundaries;
//   - randomized long haystacks over the full byte range with a fixed
//     seed, probing a present byte, an often-absent byte, and both ends.
//
// It also pins the perf premise that NO side allocates — escape analysis
// keeps the one-element needle slice on the stack — so the entire win is
// instruction count, and the comparison stays like-for-like.

import (
	"bytes"
	"math/rand"
	"testing"
)

// ps5014Before* are the exact Before-shapes of the check: a one-byte
// needle through Contains, in both the composite literal and the
// constant-string conversion spelling, plus the direct !Contains form the
// fix absorbs.
func ps5014Before(b []byte, c byte) bool            { return bytes.Contains(b, []byte{c}) }
func ps5014BeforeConv(b []byte, sub string) bool    { return bytes.Contains(b, []byte(sub)) }
func ps5014BeforeNot(b []byte, c byte) bool         { return !bytes.Contains(b, []byte{c}) }
func ps5014BeforeNotConv(b []byte, sub string) bool { return !bytes.Contains(b, []byte(sub)) }

// ps5014After* are the exact After-shapes of the check: the composite
// element passed through verbatim under >= 0, the conversion's literal
// wrapped as sub[0], the absorbed negation as < 0, and the parenthesized
// spelling the fix emits inside comparison operands.
func ps5014After(b []byte, c byte) bool           { return bytes.IndexByte(b, c) >= 0 }
func ps5014AfterLit(b []byte, sub string) bool    { return bytes.IndexByte(b, sub[0]) >= 0 }
func ps5014AfterNot(b []byte, c byte) bool        { return bytes.IndexByte(b, c) < 0 }
func ps5014AfterNotLit(b []byte, sub string) bool { return bytes.IndexByte(b, sub[0]) < 0 }
func ps5014AfterParen(b []byte, c byte, x bool) bool {
	return x == (bytes.IndexByte(b, c) >= 0)
}

func ps5014Check(t *testing.T, b []byte, c byte) {
	t.Helper()
	// string([]byte{c}), NOT string(c): the integer conversion would
	// UTF-8-encode c and yield a TWO-byte needle for c >= 0x80. The
	// check's conversion needle is a constant of decoded byte-length
	// exactly 1, so its runtime counterpart is the raw one-byte string.
	sub := string([]byte{c})
	if len(sub) != 1 {
		t.Fatalf("harness bug: needle %q is %d bytes, want 1", sub, len(sub))
	}
	after := ps5014After(b, c)
	if before := ps5014Before(b, c); before != after {
		t.Fatalf("Contains composite divergence on (%q, %#02x): before=%v after=%v", b, c, before, after)
	}
	if before := ps5014BeforeConv(b, sub); before != after {
		t.Fatalf("Contains conversion divergence on (%q, %q): before=%v after=%v", b, sub, before, after)
	}
	if lit := ps5014AfterLit(b, sub); lit != after {
		t.Fatalf("IndexByte sub[0] spelling divergence on (%q, %q): lit=%v after=%v", b, sub, lit, after)
	}
	notAfter := ps5014AfterNot(b, c)
	if notAfter != !after {
		t.Fatalf("negation identity broken on (%q, %#02x): < 0 gave %v, want %v", b, c, notAfter, !after)
	}
	if before := ps5014BeforeNot(b, c); before != notAfter {
		t.Fatalf("!Contains composite divergence on (%q, %#02x): before=%v after=%v", b, c, before, notAfter)
	}
	if before := ps5014BeforeNotConv(b, sub); before != notAfter {
		t.Fatalf("!Contains conversion divergence on (%q, %q): before=%v after=%v", b, sub, before, notAfter)
	}
	if lit := ps5014AfterNotLit(b, sub); lit != notAfter {
		t.Fatalf("IndexByte sub[0] < 0 spelling divergence on (%q, %q): lit=%v after=%v", b, sub, lit, notAfter)
	}
	// The parenthesized comparison-operand spelling is the same value in
	// both truth contexts.
	for _, x := range []bool{false, true} {
		if got, want := ps5014AfterParen(b, c, x), x == ps5014Before(b, c); got != want {
			t.Fatalf("parenthesized operand divergence on (%q, %#02x, %v): got %v want %v", b, c, x, got, want)
		}
	}
}

// ps5014Alphabet stresses every comparison hazard: ASCII, NUL, the bytes
// of the two-byte U+00E9 (C3 A9) and three-byte U+3042 (E3 81 82) so
// truncated and complete multi-byte sequences arise at every alignment,
// a bare continuation byte, and 0xFF (never valid UTF-8).
var ps5014Alphabet = []byte{'a', 'z', 0x00, 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF}

func TestEquiv_PS5014_ExhaustiveShort(t *testing.T) {
	var enumerate func(buf []byte, depth int, emit func([]byte))
	enumerate = func(buf []byte, depth int, emit func([]byte)) {
		if depth == len(buf) {
			emit(bytes.Clone(buf))
			return
		}
		for _, c := range ps5014Alphabet {
			buf[depth] = c
			enumerate(buf, depth+1, emit)
		}
	}
	var haystacks [][]byte
	for n := 0; n <= 3; n++ {
		enumerate(make([]byte, n), 0, func(b []byte) { haystacks = append(haystacks, b) })
	}
	for _, b := range haystacks {
		// ALL 256 needle bytes: absent, present once, present repeatedly,
		// at either boundary — every combination arises from the cross
		// product.
		for c := 0; c < 256; c++ {
			ps5014Check(t, b, byte(c))
		}
	}
}

func TestEquiv_PS5014_TargetedSeeds(t *testing.T) {
	long := bytes.Repeat([]byte("payload="), 4096)
	seeds := [][]byte{
		nil,
		{},
		[]byte("z"),
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
	probes := []byte{'z', 'a', '=', 0x00, 0xC3, 0xA9, 0xFF}
	for _, b := range seeds {
		for _, c := range probes {
			ps5014Check(t, b, c)
		}
	}
}

func TestEquiv_PS5014_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25014014))
	randBytes := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return b
	}
	for range 50000 {
		b := randBytes(rng.Intn(257))
		// A fully random byte (usually present), a byte from a narrow
		// range (often absent), and — when b is non-empty — its first
		// and last bytes (guaranteed hits at the boundaries).
		ps5014Check(t, b, byte(rng.Intn(256)))
		ps5014Check(t, b, byte(rng.Intn(4)))
		if len(b) > 0 {
			ps5014Check(t, b, b[0])
			ps5014Check(t, b, b[len(b)-1])
		}
	}
}

// TestEquiv_PS5014_BothSidesZeroAlloc pins the perf premise: neither the
// Before shapes (whose one-element needle slice escape analysis keeps on
// the stack) nor the After shapes allocate — the entire win is
// instruction count (the needle construction, the needle-length dispatch,
// and the sep[0] load), not allocation behavior.
func TestEquiv_PS5014_BothSidesZeroAlloc(t *testing.T) {
	b := []byte("service=checkout region=eu-west-1 shard=07 status=ok final=z")
	c := byte('z')
	var sink bool
	allocs := testing.AllocsPerRun(100, func() {
		sink = bytes.Contains(b, []byte{c})
		sink = sink != bytes.Contains(b, []byte("z"))
		sink = sink != !bytes.Contains(b, []byte{c})
		sink = sink != (bytes.IndexByte(b, c) >= 0)
		sink = sink != (bytes.IndexByte(b, "z"[0]) >= 0)
		sink = sink != (bytes.IndexByte(b, c) < 0)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the Contains/IndexByte sextet allocated %v times per run; PS5014's like-for-like premise no longer holds", allocs)
	}
}
