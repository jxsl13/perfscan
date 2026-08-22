package checks

// Runtime differential for PS5007: strings.Index(s, sub) vs
// strings.IndexByte(s, sub[0]) — and the LastIndex/LastIndexByte
// sibling — for every ONE-BYTE needle sub. The fix's safety argument is
// that for len(sub) == 1 both functions perform the identical raw
// byte-wise search (Index is documented and implemented as delegating to
// IndexByte for a one-byte needle; LastIndex compares raw bytes
// backwards) with no rune decoding, no UTF-8 validation, and no case
// folding anywhere — so the returned index is equal for every haystack.
// The sub[0] spelling mirrors the fix's literal wrap ("z"[0]) exactly.
// This suite pins that claim over:
//
//   - EXHAUSTIVE pairs: every haystack of length <= 3 over an adversarial
//     alphabet (ASCII, NUL, the bytes of multi-byte runes so truncated
//     and complete sequences arise at every alignment, a bare
//     continuation byte, and 0xFF) crossed with ALL 256 possible needle
//     bytes — first/last occurrence, absence, and boundary positions all
//     arise from the cross product;
//   - targeted seeds: empty haystack, needle only at the start, only at
//     the end, repeated (first vs last occurrence differ), NUL and 0xFF
//     needles inside invalid UTF-8, and long haystacks that cross the
//     SIMD-scan block boundaries;
//   - randomized long haystacks over the full byte range with a fixed
//     seed, probing a present byte, an often-absent byte, and both ends.
//
// It also pins the perf premise that neither side allocates: the win is
// pure instruction count, so any allocation on either side would mean
// the comparison is no longer like-for-like.

import (
	"math/rand"
	"strings"
	"testing"
)

// ps5007Before* are the exact Before-shapes of the check (a one-byte
// string needle through the generic substring search).
func ps5007BeforeIndex(s, sub string) int     { return strings.Index(s, sub) }
func ps5007BeforeLastIndex(s, sub string) int { return strings.LastIndex(s, sub) }

// ps5007After* are the exact After-shapes of the check: the needle
// literal wrapped as sub[0], precisely what the fix splices in.
func ps5007AfterIndex(s, sub string) int     { return strings.IndexByte(s, sub[0]) }
func ps5007AfterLastIndex(s, sub string) int { return strings.LastIndexByte(s, sub[0]) }

func ps5007Check(t *testing.T, s string, c byte) {
	t.Helper()
	// string([]byte{c}), NOT string(c): the integer conversion would
	// UTF-8-encode c and yield a TWO-byte needle for c >= 0x80. The
	// check's needle is a literal of decoded byte-length exactly 1, so
	// its runtime counterpart is the raw one-byte string.
	sub := string([]byte{c})
	if len(sub) != 1 {
		t.Fatalf("harness bug: needle %q is %d bytes, want 1", sub, len(sub))
	}
	if before, after := ps5007BeforeIndex(s, sub), ps5007AfterIndex(s, sub); before != after {
		t.Fatalf("Index divergence on (%q, %q): before=%d after=%d", s, sub, before, after)
	}
	if before, after := ps5007BeforeLastIndex(s, sub), ps5007AfterLastIndex(s, sub); before != after {
		t.Fatalf("LastIndex divergence on (%q, %q): before=%d after=%d", s, sub, before, after)
	}
}

// ps5007Alphabet stresses every comparison hazard: ASCII, NUL, the bytes
// of the two-byte U+00E9 (C3 A9) and three-byte U+3042 (E3 81 82) so
// truncated and complete multi-byte sequences arise at every alignment,
// a bare continuation byte, and 0xFF (never valid UTF-8).
var ps5007Alphabet = []byte{'a', 'z', 0x00, 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF}

func TestEquiv_PS5007_ExhaustiveShort(t *testing.T) {
	var enumerate func(buf []byte, depth int, emit func(string))
	enumerate = func(buf []byte, depth int, emit func(string)) {
		if depth == len(buf) {
			emit(string(buf))
			return
		}
		for _, c := range ps5007Alphabet {
			buf[depth] = c
			enumerate(buf, depth+1, emit)
		}
	}
	var haystacks []string
	for n := 0; n <= 3; n++ {
		enumerate(make([]byte, n), 0, func(s string) { haystacks = append(haystacks, s) })
	}
	for _, s := range haystacks {
		// ALL 256 needle bytes: absent, present once, present repeatedly
		// (where first and last occurrence differ), at either boundary —
		// every combination arises from the cross product.
		for c := 0; c < 256; c++ {
			ps5007Check(t, s, byte(c))
		}
	}
}

func TestEquiv_PS5007_TargetedSeeds(t *testing.T) {
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
		strings.Repeat("z", 1000), // needle everywhere: first and last differ maximally
		long,                      // long, needle absent
		long + "z",                // long, needle only at the very end
		"z" + long,                // long, needle only at the very start
		long + "z" + long,         // long, needle in the middle
	}
	probes := []byte{'z', 'a', '=', 0x00, 0xC3, 0xA9, 0xFF}
	for _, s := range seeds {
		for _, c := range probes {
			ps5007Check(t, s, c)
		}
	}
}

func TestEquiv_PS5007_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25007007))
	randBytes := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return string(b)
	}
	for range 50000 {
		s := randBytes(rng.Intn(257))
		// A fully random byte (usually present), a byte from a narrow
		// range (often absent), and — when s is non-empty — its first
		// and last bytes (guaranteed boundary hits).
		ps5007Check(t, s, byte(rng.Intn(256)))
		ps5007Check(t, s, byte(rng.Intn(4)))
		if len(s) > 0 {
			ps5007Check(t, s, s[0])
			ps5007Check(t, s, s[len(s)-1])
		}
	}
}

// TestEquiv_PS5007_BothSidesZeroAlloc pins the perf premise: neither the
// Before nor the After shape allocates — the entire win is instruction
// count (the needle-length dispatch and, for LastIndex, the whole
// Rabin-Karp reverse search), not allocation behavior.
func TestEquiv_PS5007_BothSidesZeroAlloc(t *testing.T) {
	s := "service=checkout region=eu-west-1 shard=07 status=ok final=z"
	var sink int
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.Index(s, "z")
		sink += strings.LastIndex(s, "z")
		sink += strings.IndexByte(s, "z"[0])
		sink += strings.LastIndexByte(s, "z"[0])
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the Index/IndexByte quartet allocated %v times per run; PS5007's like-for-like premise no longer holds", allocs)
	}
}
