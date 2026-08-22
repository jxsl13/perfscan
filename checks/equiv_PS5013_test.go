package checks

// Runtime differential for PS5013: bytes.Index(b, []byte{c}) /
// bytes.Index(b, []byte(sub)) vs bytes.IndexByte(b, c) — and the
// LastIndex/LastIndexByte siblings — for every ONE-BYTE needle. The fix's
// safety argument is that for len(sep) == 1 both functions perform the
// identical raw byte-wise search: bytes.Index is implemented as
// IndexByte(s, sep[0]) for a one-byte needle, and bytes.LastIndex as
// bytealg.LastIndexByte(s, sep[0]) — the exact function
// bytes.LastIndexByte wraps — with no rune decoding, no UTF-8 validation,
// and no case folding anywhere, so the returned index is equal for every
// haystack. Both Before spellings the check rewrites are exercised (the
// composite literal []byte{c} and the conversion []byte(sub) with the
// sub[0] wrap the fix emits). This suite pins that claim over:
//
//   - EXHAUSTIVE pairs: every haystack of length <= 3 over an adversarial
//     alphabet (ASCII, NUL, the bytes of multi-byte runes so truncated
//     and complete sequences arise at every alignment, a bare
//     continuation byte, and 0xFF) crossed with ALL 256 possible needle
//     bytes — first/last occurrence, absence, and boundary positions all
//     arise from the cross product;
//   - targeted seeds: nil and empty haystacks, needle only at the start,
//     only at the end, repeated (first vs last occurrence differ), NUL
//     and 0xFF needles inside invalid UTF-8, and long haystacks that
//     cross the SIMD-scan block boundaries;
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

// ps5013Before* are the exact Before-shapes of the check: a one-byte
// needle through the generic substring search, in both the composite
// literal and the constant-string conversion spelling.
func ps5013BeforeIndex(b []byte, c byte) int         { return bytes.Index(b, []byte{c}) }
func ps5013BeforeLastIndex(b []byte, c byte) int     { return bytes.LastIndex(b, []byte{c}) }
func ps5013BeforeIndexConv(b []byte, sub string) int { return bytes.Index(b, []byte(sub)) }
func ps5013BeforeLastIndexConv(b []byte, sub string) int {
	return bytes.LastIndex(b, []byte(sub))
}

// ps5013After* are the exact After-shapes of the check: the composite
// element passed through verbatim, and the conversion's literal wrapped
// as sub[0] — precisely what the fix splices in.
func ps5013AfterIndex(b []byte, c byte) int            { return bytes.IndexByte(b, c) }
func ps5013AfterLastIndex(b []byte, c byte) int        { return bytes.LastIndexByte(b, c) }
func ps5013AfterIndexLit(b []byte, sub string) int     { return bytes.IndexByte(b, sub[0]) }
func ps5013AfterLastIndexLit(b []byte, sub string) int { return bytes.LastIndexByte(b, sub[0]) }

func ps5013Check(t *testing.T, b []byte, c byte) {
	t.Helper()
	// string([]byte{c}), NOT string(c): the integer conversion would
	// UTF-8-encode c and yield a TWO-byte needle for c >= 0x80. The
	// check's conversion needle is a constant of decoded byte-length
	// exactly 1, so its runtime counterpart is the raw one-byte string.
	sub := string([]byte{c})
	if len(sub) != 1 {
		t.Fatalf("harness bug: needle %q is %d bytes, want 1", sub, len(sub))
	}
	after := ps5013AfterIndex(b, c)
	if before := ps5013BeforeIndex(b, c); before != after {
		t.Fatalf("Index composite divergence on (%q, %#02x): before=%d after=%d", b, c, before, after)
	}
	if before := ps5013BeforeIndexConv(b, sub); before != after {
		t.Fatalf("Index conversion divergence on (%q, %q): before=%d after=%d", b, sub, before, after)
	}
	if lit := ps5013AfterIndexLit(b, sub); lit != after {
		t.Fatalf("IndexByte sub[0] spelling divergence on (%q, %q): lit=%d after=%d", b, sub, lit, after)
	}
	lastAfter := ps5013AfterLastIndex(b, c)
	if before := ps5013BeforeLastIndex(b, c); before != lastAfter {
		t.Fatalf("LastIndex composite divergence on (%q, %#02x): before=%d after=%d", b, c, before, lastAfter)
	}
	if before := ps5013BeforeLastIndexConv(b, sub); before != lastAfter {
		t.Fatalf("LastIndex conversion divergence on (%q, %q): before=%d after=%d", b, sub, before, lastAfter)
	}
	if lit := ps5013AfterLastIndexLit(b, sub); lit != lastAfter {
		t.Fatalf("LastIndexByte sub[0] spelling divergence on (%q, %q): lit=%d after=%d", b, sub, lit, lastAfter)
	}
}

// ps5013Alphabet stresses every comparison hazard: ASCII, NUL, the bytes
// of the two-byte U+00E9 (C3 A9) and three-byte U+3042 (E3 81 82) so
// truncated and complete multi-byte sequences arise at every alignment,
// a bare continuation byte, and 0xFF (never valid UTF-8).
var ps5013Alphabet = []byte{'a', 'z', 0x00, 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF}

func TestEquiv_PS5013_ExhaustiveShort(t *testing.T) {
	var enumerate func(buf []byte, depth int, emit func([]byte))
	enumerate = func(buf []byte, depth int, emit func([]byte)) {
		if depth == len(buf) {
			emit(bytes.Clone(buf))
			return
		}
		for _, c := range ps5013Alphabet {
			buf[depth] = c
			enumerate(buf, depth+1, emit)
		}
	}
	var haystacks [][]byte
	for n := 0; n <= 3; n++ {
		enumerate(make([]byte, n), 0, func(b []byte) { haystacks = append(haystacks, b) })
	}
	for _, b := range haystacks {
		// ALL 256 needle bytes: absent, present once, present repeatedly
		// (where first and last occurrence differ), at either boundary —
		// every combination arises from the cross product.
		for c := 0; c < 256; c++ {
			ps5013Check(t, b, byte(c))
		}
	}
}

func TestEquiv_PS5013_TargetedSeeds(t *testing.T) {
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
		bytes.Repeat([]byte("z"), 1000),                 // needle everywhere: first and last differ maximally
		long,                                            // long, needle absent
		append(bytes.Clone(long), 'z'),                  // long, needle only at the very end
		append([]byte("z"), long...),                    // long, needle only at the very start
		append(append(bytes.Clone(long), 'z'), long...), // long, needle in the middle
	}
	probes := []byte{'z', 'a', '=', 0x00, 0xC3, 0xA9, 0xFF}
	for _, b := range seeds {
		for _, c := range probes {
			ps5013Check(t, b, c)
		}
	}
}

func TestEquiv_PS5013_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25013013))
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
		// and last bytes (guaranteed boundary hits).
		ps5013Check(t, b, byte(rng.Intn(256)))
		ps5013Check(t, b, byte(rng.Intn(4)))
		if len(b) > 0 {
			ps5013Check(t, b, b[0])
			ps5013Check(t, b, b[len(b)-1])
		}
	}
}

// TestEquiv_PS5013_BothSidesZeroAlloc pins the perf premise: neither the
// Before shapes (whose one-element needle slice escape analysis keeps on
// the stack) nor the After shapes allocate — the entire win is
// instruction count (the needle construction, the needle-length dispatch,
// and the sep[0] load), not allocation behavior.
func TestEquiv_PS5013_BothSidesZeroAlloc(t *testing.T) {
	b := []byte("service=checkout region=eu-west-1 shard=07 status=ok final=z")
	c := byte('z')
	var sink int
	allocs := testing.AllocsPerRun(100, func() {
		sink = bytes.Index(b, []byte{c})
		sink += bytes.LastIndex(b, []byte{c})
		sink += bytes.Index(b, []byte("z"))
		sink += bytes.LastIndex(b, []byte("z"))
		sink += bytes.IndexByte(b, c)
		sink += bytes.LastIndexByte(b, "z"[0])
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the Index/IndexByte sextet allocated %v times per run; PS5013's like-for-like premise no longer holds", allocs)
	}
}
