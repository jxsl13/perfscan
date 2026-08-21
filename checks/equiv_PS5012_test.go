package checks

// Runtime differential for PS5012:
// strings.Replace(s, old, new, strings.Count(s, old)) vs
// strings.ReplaceAll(s, old, new). The fix's safety argument is that
// Replace clamps its cap to m = Count(s, old) and then replaces exactly
// min(n, m) leftmost non-overlapping occurrences, so passing n == m
// replaces all m of them — precisely what ReplaceAll's n = -1 resolves
// to. Count and Replace share the same non-overlapping left-to-right
// match walk (both byte-oriented), so the explicit count can never
// overshoot or undershoot the sites Replace visits. The equality holds
// on every edge the API has: no match (m == 0, both return s itself),
// old == new, self-overlapping patterns, and old == "" — where
// Count(s, "") is utf8.RuneCountInString(s)+1, exactly the k+1 boundary
// insertions ReplaceAll performs, with identical invalid-UTF-8
// treatment (RuneError width 1). This suite pins that claim over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 3 over an
//     adversarial alphabet (ASCII, NUL, the bytes of a multi-byte rune
//     so truncated and complete sequences both arise, and 0xFF),
//     crossed with old/new sets holding empty strings, single and
//     multi-byte ASCII, multi-byte runes, U+FFFD itself, raw invalid
//     UTF-8, and self-overlapping patterns;
//   - targeted seeds (empty-old insertion over invalid UTF-8, adjacent
//     matches, old == new, deletions, long inputs with periodic
//     matches and with no match at all);
//   - randomized long inputs over the full byte range with a fixed
//     seed.
//
// It also pins the perf premise itself: the Before really does walk s
// twice — its allocation profile is identical to the After (one result
// allocation, zero on no match), so the whole delta is the redundant
// explicit Count scan, which the benchmark pair in
// benchmarks/ps5012_test.go measures.

import (
	"math/rand"
	"strings"
	"testing"
)

// ps5012Before is the exact Before-shape of the check.
func ps5012Before(s, old, new string) string {
	return strings.Replace(s, old, new, strings.Count(s, old))
}

// ps5012After is the exact After-shape of the check.
func ps5012After(s, old, new string) string {
	return strings.ReplaceAll(s, old, new)
}

func ps5012Check(t *testing.T, s, old, new string) {
	t.Helper()
	if before, after := ps5012Before(s, old, new), ps5012After(s, old, new); before != after {
		t.Fatalf("divergence on (%q, %q, %q):\n before=%q\n after=%q", s, old, new, before, after)
	}
}

// ps5012Pairs exercises every matching path: the empty-old rune
// insertion, single-byte and multi-byte literals, self-overlapping
// patterns, U+FFFD vs raw invalid UTF-8, and NUL.
var ps5012Pairs = []string{
	"",         // empty old: Count = RuneCount+1 = the k+1 insertions of ReplaceAll
	"a",        // single ASCII byte
	"aa",       // self-overlapping candidate
	"ab",       // multi-byte ASCII literal
	"\x00",     // NUL
	"é",        // two-byte rune
	"あ",        // three-byte rune
	"�",        // U+FFFD literally: must NOT match raw invalid sequences
	"\xff",     // invalid UTF-8
	"\xc3",     // lead byte of é alone — a truncated sequence
	"\xc3\xa9", // the é bytes spelled raw
	"ZZ",       // longer replacement than most inputs
}

func TestEquiv_PS5012_ExhaustiveShort(t *testing.T) {
	alphabet := []byte{
		'a', 'b', // ASCII match/non-match material
		0x00,       // NUL
		0xC3, 0xA9, // C3 A9 = U+00E9; each byte alone is invalid UTF-8
		0xFF, // never valid in UTF-8
	}
	var buf [3]byte
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			s := string(buf[:n])
			for _, old := range ps5012Pairs {
				for _, new := range ps5012Pairs {
					ps5012Check(t, s, old, new)
				}
			}
			return
		}
		for _, c := range alphabet {
			buf[depth] = c
			rec(n, depth+1)
		}
	}
	for n := 0; n <= len(buf); n++ {
		rec(n, 0)
	}
}

func TestEquiv_PS5012_TargetedSeeds(t *testing.T) {
	long := strings.Repeat("key=val;", 4096)
	type seed struct{ s, old, new string }
	seeds := []seed{
		{"", "", "-"},                            // empty everything: Count("", "") == 1, one insertion
		{"", "a", "b"},                           // empty s, no match: Count == 0
		{"aaa", "a", "bb"},                       // every byte replaced, result grows
		{"aaaa", "aa", "b"},                      // overlap: Count == 2 == the two non-overlapping replaces
		{"hello world", "o", "0"},                // plain
		{"hello", "x", "y"},                      // NO match: Count == 0, both return s
		{"aXbXc", "X", ""},                       // deletion
		{"aXbXc", "X", "X"},                      // old == new
		{"swap", "a", "o"},                       // len(old) == len(new) == 1 byte swap
		{"\xff\xfe", "", "."},                    // empty old over invalid UTF-8: RuneError width 1 each
		{"\xc3\xa9\xc3", "\xc3", "?"},            // raw lead byte matches inside AND after a valid rune
		{"straße", "ß", "ss"},                    // multi-byte old, ASCII new
		{"a\x00b\x00c", "\x00", " "},             // NUL replacement
		{"ééé", "é", "e"},                        // adjacent multi-byte matches
		{"�x�", "�", "!"},                        // literal U+FFFD only, raw invalid must not match
		{long, "key", "KEY"},                     // long input, periodic matches
		{long, ";", ""},                          // long input, shrinking result
		{long, "absent", "-"},                    // long input, no match
		{long, "", "|"},                          // long input, empty-old rune insertion
		{strings.Repeat("ab", 8192), "ab", "ba"}, // long full-coverage replacement
	}
	for _, sd := range seeds {
		ps5012Check(t, sd.s, sd.old, sd.new)
	}
}

func TestEquiv_PS5012_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25012))
	randBytes := func(n int, pick func() byte) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = pick()
		}
		return string(b)
	}
	// Full byte range for all three operands: mostly invalid UTF-8 at
	// random alignments, old lengths flipping between the empty-old rune
	// path and the substring-search path at random.
	anyByte := func() byte { return byte(rng.Intn(256)) }
	for range 50000 {
		s := randBytes(rng.Intn(40), anyByte)
		old := randBytes(rng.Intn(4), anyByte)
		new := randBytes(rng.Intn(5), anyByte)
		ps5012Check(t, s, old, new)
	}
	// Match-heavy alphabet so replacements routinely hit, overlap, and
	// straddle multi-byte rune boundaries.
	heavy := []byte{'a', 'b', 0xC3, 0xA9, 0xFF}
	heavyByte := func() byte { return heavy[rng.Intn(len(heavy))] }
	for range 50000 {
		s := randBytes(rng.Intn(30), heavyByte)
		old := randBytes(rng.Intn(3), heavyByte)
		new := randBytes(rng.Intn(4), heavyByte)
		ps5012Check(t, s, old, new)
	}
}

// TestEquiv_PS5012_AllocParity pins that the rewrite is pure work
// removal: the Before and After allocate identically (one result
// allocation on a match, ZERO on no match — Replace returns s itself
// when its internal count is zero), so the benchmark delta is entirely
// the redundant explicit Count scan.
func TestEquiv_PS5012_AllocParity(t *testing.T) {
	s := "a payload that is comfortably long enough to heap-allocate on copy"
	var sink string
	for _, old := range []string{"absent", "o"} {
		before := testing.AllocsPerRun(100, func() {
			sink = strings.Replace(s, old, "-", strings.Count(s, old))
		})
		after := testing.AllocsPerRun(100, func() {
			sink = strings.ReplaceAll(s, old, "-")
		})
		if before != after {
			t.Fatalf("alloc profile diverged for old=%q: before %v, after %v — PS5012's pure-work-removal premise no longer holds", old, before, after)
		}
	}
	_ = sink
}
