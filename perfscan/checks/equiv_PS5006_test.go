package checks

// Runtime differential for PS5006:
// string(bytes.TrimPrefix([]byte(s), []byte(p))) vs
// strings.TrimPrefix(s, p), plus the TrimSuffix sibling. The fix's
// safety argument is that both packages share identical semantics: a
// pure byte-wise prefix (resp. suffix) comparison — a length check plus
// bytewise equality — followed by reslicing off the matched span, with
// no rune decoding, no case folding, and no Unicode normalization
// anywhere. So the resulting string VALUE is identical for every
// (s, p) pair. This suite pins that claim over:
//
//   - EXHAUSTIVE short pairs: every byte string of length <= 3 over an
//     adversarial alphabet (ASCII, NUL, the bytes of multi-byte runes so
//     truncated and complete sequences both arise, a bare continuation
//     byte, and 0xFF) crossed with EVERY string of length <= 2 over the
//     same alphabet as the prefix/suffix — plus, for each s, every
//     prefix of s and every suffix of s (the always-matching cases,
//     including p == s);
//   - targeted seeds (empty s, empty p, p longer than s, p differing
//     only in the last byte, multi-byte UTF-8 split across the
//     boundary, invalid UTF-8 on both sides, NUL bytes, very long
//     inputs);
//   - randomized long inputs over the full byte range with a fixed
//     seed, where the prefix/suffix is taken from s half the time so
//     matches actually occur.
//
// It also pins the perf premise: the strings twins perform zero
// allocations (they return a substring), which is the entire win.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps5006Before* are the exact Before-shapes of the check.
func ps5006BeforePrefix(s, p string) string {
	return string(bytes.TrimPrefix([]byte(s), []byte(p)))
}

func ps5006BeforeSuffix(s, p string) string {
	return string(bytes.TrimSuffix([]byte(s), []byte(p)))
}

// ps5006After* are the exact After-shapes of the check.
func ps5006AfterPrefix(s, p string) string { return strings.TrimPrefix(s, p) }
func ps5006AfterSuffix(s, p string) string { return strings.TrimSuffix(s, p) }

func ps5006Check(t *testing.T, s, p string) {
	t.Helper()
	if before, after := ps5006BeforePrefix(s, p), ps5006AfterPrefix(s, p); before != after {
		t.Fatalf("TrimPrefix divergence on (%q, %q):\n before=%q\n after=%q", s, p, before, after)
	}
	if before, after := ps5006BeforeSuffix(s, p), ps5006AfterSuffix(s, p); before != after {
		t.Fatalf("TrimSuffix divergence on (%q, %q):\n before=%q\n after=%q", s, p, before, after)
	}
}

// ps5006Alphabet stresses every comparison hazard: ASCII, NUL, the
// bytes of the two-byte U+00E9 (C3 A9) and three-byte U+3042 (E3 81 82)
// so truncated and complete multi-byte sequences arise at every
// alignment, a bare continuation byte, and 0xFF (never valid UTF-8).
var ps5006Alphabet = []byte{'a', '-', 0x00, 0xC3, 0xA9, 0xE3, 0x81, 0x82, 0xFF}

func TestEquiv_PS5006_ExhaustiveShort(t *testing.T) {
	var enumerate func(buf []byte, depth int, emit func(string))
	enumerate = func(buf []byte, depth int, emit func(string)) {
		if depth == len(buf) {
			emit(string(buf))
			return
		}
		for _, c := range ps5006Alphabet {
			buf[depth] = c
			enumerate(buf, depth+1, emit)
		}
	}
	var ss, ps []string
	for n := 0; n <= 3; n++ {
		enumerate(make([]byte, n), 0, func(s string) { ss = append(ss, s) })
	}
	for n := 0; n <= 2; n++ {
		enumerate(make([]byte, n), 0, func(p string) { ps = append(ps, p) })
	}
	for _, s := range ss {
		// Every short p: matching, non-matching, longer-than-s, and
		// almost-matching pairs all arise from the cross product.
		for _, p := range ps {
			ps5006Check(t, s, p)
		}
		// Every prefix and every suffix of s: the always-matching cases,
		// including p == s (fully trimmed to "").
		for i := 0; i <= len(s); i++ {
			ps5006Check(t, s, s[:i])
			ps5006Check(t, s, s[i:])
		}
	}
}

func TestEquiv_PS5006_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("payload/", 4096)
	pairs := [][2]string{
		{"", ""},
		{"", "x"},                       // p longer than s
		{"x", ""},                       // empty p: s returned unchanged
		{"x", "x"},                      // p == s: trimmed to "" (bytes yields an empty slice)
		{"x", "xx"},                     // p longer than s, sharing a prefix
		{"prefix-body", "prefix-"},      // plain match
		{"prefix-body", "prefix_"},      // differs in the last byte of p
		{"body-suffix", "-suffix"},      // plain suffix match
		{"body-suffix", "_suffix"},      // differs in the first byte of p
		{"ééx", "é"},                    // multi-byte rune prefix
		{"xéé", "é"},                    // multi-byte rune suffix
		{"\xc3\xa9x", "\xc3"},           // p is a TRUNCATED multi-byte sequence: byte-wise match splits the rune
		{"x\xc3\xa9", "\xa9"},           // suffix match on a bare continuation byte
		{"\xff\xfex", "\xff"},           // invalid UTF-8 in both s and p
		{"x\x80", "\x80"},               // bare continuation byte suffix
		{"\x00ab\x00", "\x00"},          // NUL prefix and suffix
		{"a\x00b", "a\x00"},             // NUL inside the matched span
		{"��", "�"},                     // literal replacement chars
		{long + "tail", long},           // long matching prefix
		{"head" + long, long},           // long matching suffix
		{long, long},                    // long p == s
		{long + "x", long + "y"},        // long p diverging at the last byte
		{"\xe3\x81\x82abc", "\xe3\x81"}, // three-byte rune split by a byte-wise prefix match
	}
	for _, pr := range pairs {
		ps5006Check(t, pr[0], pr[1])
		ps5006Check(t, pr[1], pr[0]) // both orientations
	}
}

func TestEquiv_PS5006_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25006006))
	randBytes := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return string(b)
	}
	for range 100000 {
		s := randBytes(rng.Intn(65))
		var p string
		if rng.Intn(2) == 0 || len(s) == 0 {
			// Independent random p: overwhelmingly non-matching.
			p = randBytes(rng.Intn(9))
		} else if rng.Intn(2) == 0 {
			// A real prefix of s: guaranteed TrimPrefix hit.
			p = s[:rng.Intn(len(s)+1)]
		} else {
			// A real suffix of s: guaranteed TrimSuffix hit.
			p = s[rng.Intn(len(s)+1):]
		}
		ps5006Check(t, s, p)
	}
}

// TestEquiv_PS5006_AfterIsZeroAlloc pins the perf premise: the rewritten
// forms allocate nothing (they return a substring of the input), which
// is exactly the win the check promises. The Before-form's mandatory
// string(...) result copy is measured in benchmarks/ps5006_test.go.
func TestEquiv_PS5006_AfterIsZeroAlloc(t *testing.T) {
	s := "prefix: a payload that is clearly long enough to heap-allocate :suffix"
	const pre = "prefix: "
	const suf = " :suffix"
	var sink string
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.TrimPrefix(s, pre)
		sink = strings.TrimSuffix(s, suf)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the strings TrimPrefix/TrimSuffix pair allocated %v times per run; the zero-copy premise of PS5006 no longer holds", allocs)
	}
}
