package checks

// Runtime differential for PS5019 (the bytes twin of PS5012):
// bytes.Replace(b, old, new, bytes.Count(b, old)) vs
// bytes.ReplaceAll(b, old, new). The fix's safety argument is that
// Replace clamps its cap to m = Count(b, old) and then replaces exactly
// min(n, m) leftmost non-overlapping occurrences, so passing n == m
// replaces all m of them — precisely what ReplaceAll's n = -1 resolves
// to. Count and Replace share the same non-overlapping left-to-right
// match walk (both byte-oriented), so the explicit count can never
// overshoot or undershoot the sites Replace visits. The equality holds
// on every edge the API has: no match (m == 0 — both forms return a
// FRESH copy of b, never b itself, so there is no aliasing wrinkle:
// the differential pins that neither result ever shares memory with
// the input), old == new, self-overlapping patterns, and old ==
// nil/empty — where Count(b, nil) is utf8.RuneCount(b)+1, exactly the
// k+1 boundary insertions ReplaceAll performs, with identical
// invalid-UTF-8 treatment (RuneError width 1). This suite pins that
// claim over:
//
//   - EXHAUSTIVE short inputs: every byte slice of length <= 3 over an
//     adversarial alphabet (ASCII, NUL, the bytes of a multi-byte rune
//     so truncated and complete sequences both arise, and 0xFF),
//     crossed with old/new sets holding nil, empty, single and
//     multi-byte ASCII, multi-byte runes, U+FFFD itself, raw invalid
//     UTF-8, and self-overlapping patterns;
//   - targeted seeds (empty-old insertion over invalid UTF-8, adjacent
//     matches, old == new, deletions, long inputs with periodic
//     matches and with no match at all);
//   - randomized long inputs over the full byte range with a fixed
//     seed.
//
// It also pins the perf premise itself: the Before really does walk b
// twice — its allocation profile is identical to the After (exactly one
// result allocation, on match AND on no-match alike, since bytes.Replace
// always returns a fresh copy), so the whole delta is the redundant
// explicit Count scan, which the benchmark pair in
// benchmarks/ps5019_test.go measures.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps5019Before is the exact Before-shape of the check.
func ps5019Before(b, old, new []byte) []byte {
	return bytes.Replace(b, old, new, bytes.Count(b, old))
}

// ps5019After is the exact After-shape of the check.
func ps5019After(b, old, new []byte) []byte {
	return bytes.ReplaceAll(b, old, new)
}

// ps5019Aliases reports whether out shares its first backing byte with
// in (the only aliasing bytes.Replace could ever exhibit is returning a
// subslice of the input; comparing the address of element 0 detects
// exactly that).
func ps5019Aliases(out, in []byte) bool {
	return len(out) > 0 && len(in) > 0 && &out[0] == &in[0]
}

func ps5019Check(t *testing.T, b, old, new []byte) {
	t.Helper()
	before, after := ps5019Before(b, old, new), ps5019After(b, old, new)
	if !bytes.Equal(before, after) {
		t.Fatalf("divergence on (%q, %q, %q):\n before=%q\n after=%q", b, old, new, before, after)
	}
	// Neither form may return a slice aliasing the input: bytes.Replace
	// always copies, even when nothing matched. A future stdlib change
	// breaking that (for either form, asymmetrically) would make the
	// rewrite observable through mutation, so pin it.
	if ps5019Aliases(before, b) || ps5019Aliases(after, b) {
		t.Fatalf("aliasing on (%q, %q, %q): before aliases=%v after aliases=%v — bytes.Replace/ReplaceAll must always return a fresh copy", b, old, new, ps5019Aliases(before, b), ps5019Aliases(after, b))
	}
}

// ps5019Pairs exercises every matching path: nil and empty old (the
// rune-boundary insertion path), single-byte and multi-byte literals,
// self-overlapping patterns, U+FFFD vs raw invalid UTF-8, and NUL.
var ps5019Pairs = [][]byte{
	nil,          // nil old: Count = RuneCount+1 = the k+1 insertions of ReplaceAll
	{},           // empty non-nil old: same path, distinct header
	[]byte("a"),  // single ASCII byte
	[]byte("aa"), // self-overlapping candidate
	[]byte("ab"), // multi-byte ASCII literal
	{0x00},       // NUL
	[]byte("é"),  // two-byte rune
	[]byte("あ"),  // three-byte rune
	[]byte("�"),  // U+FFFD literally: must NOT match raw invalid sequences
	{0xFF},       // invalid UTF-8
	{0xC3},       // lead byte of é alone — a truncated sequence
	{0xC3, 0xA9}, // the é bytes spelled raw
	[]byte("ZZ"), // longer replacement than most inputs
}

func TestEquiv_PS5019_ExhaustiveShort(t *testing.T) {
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
			b := buf[:n]
			for _, old := range ps5019Pairs {
				for _, new := range ps5019Pairs {
					ps5019Check(t, b, old, new)
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
	// And the nil haystack against every pair (buf[:0] above is empty
	// but non-nil).
	for _, old := range ps5019Pairs {
		for _, new := range ps5019Pairs {
			ps5019Check(t, nil, old, new)
		}
	}
}

func TestEquiv_PS5019_TargetedSeeds(t *testing.T) {
	long := bytes.Repeat([]byte("key=val;"), 4096)
	type seed struct{ b, old, new []byte }
	bs := func(s string) []byte { return []byte(s) }
	seeds := []seed{
		{nil, nil, bs("-")},                                // nil everything: Count(nil, nil) == 1, one insertion
		{nil, bs("a"), bs("b")},                            // nil b, no match: Count == 0
		{bs(""), bs(""), bs("-")},                          // empty everything: one insertion
		{bs("aaa"), bs("a"), bs("bb")},                     // every byte replaced, result grows
		{bs("aaaa"), bs("aa"), bs("b")},                    // overlap: Count == 2 == the two non-overlapping replaces
		{bs("hello world"), bs("o"), bs("0")},              // plain
		{bs("hello"), bs("x"), bs("y")},                    // NO match: Count == 0, both return a fresh copy
		{bs("aXbXc"), bs("X"), nil},                        // deletion (nil new)
		{bs("aXbXc"), bs("X"), bs("")},                     // deletion (empty new)
		{bs("aXbXc"), bs("X"), bs("X")},                    // old == new
		{bs("swap"), bs("a"), bs("o")},                     // len(old) == len(new) == 1 byte swap
		{[]byte{0xFF, 0xFE}, nil, bs(".")},                 // nil old over invalid UTF-8: RuneError width 1 each
		{[]byte{0xC3, 0xA9, 0xC3}, []byte{0xC3}, bs("?")},  // raw lead byte matches inside AND after a valid rune
		{bs("straße"), bs("ß"), bs("ss")},                  // multi-byte old, ASCII new
		{bs("a\x00b\x00c"), bs("\x00"), bs(" ")},           // NUL replacement
		{bs("ééé"), bs("é"), bs("e")},                      // adjacent multi-byte matches
		{bs("�x�"), bs("�"), bs("!")},                      // literal U+FFFD only, raw invalid must not match
		{long, bs("key"), bs("KEY")},                       // long input, periodic matches
		{long, bs(";"), nil},                               // long input, shrinking result
		{long, bs("absent"), bs("-")},                      // long input, no match
		{long, nil, bs("|")},                               // long input, nil-old rune insertion
		{bytes.Repeat(bs("ab"), 8192), bs("ab"), bs("ba")}, // long full-coverage replacement
	}
	for _, sd := range seeds {
		ps5019Check(t, sd.b, sd.old, sd.new)
	}
}

func TestEquiv_PS5019_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25019))
	randBytes := func(n int, pick func() byte) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = pick()
		}
		return b
	}
	// Full byte range for all three operands: mostly invalid UTF-8 at
	// random alignments, old lengths flipping between the empty-old rune
	// path and the substring-search path at random.
	anyByte := func() byte { return byte(rng.Intn(256)) }
	for range 50000 {
		b := randBytes(rng.Intn(40), anyByte)
		old := randBytes(rng.Intn(4), anyByte)
		new := randBytes(rng.Intn(5), anyByte)
		ps5019Check(t, b, old, new)
	}
	// Match-heavy alphabet so replacements routinely hit, overlap, and
	// straddle multi-byte rune boundaries.
	heavy := []byte{'a', 'b', 0xC3, 0xA9, 0xFF}
	heavyByte := func() byte { return heavy[rng.Intn(len(heavy))] }
	for range 50000 {
		b := randBytes(rng.Intn(30), heavyByte)
		old := randBytes(rng.Intn(3), heavyByte)
		new := randBytes(rng.Intn(4), heavyByte)
		ps5019Check(t, b, old, new)
	}
	// Cross-check against the strings twin: on identical byte content
	// the bytes and strings pipelines must agree, so PS5019's equality
	// inherits PS5012's independently-pinned one.
	for range 5000 {
		b := randBytes(rng.Intn(30), anyByte)
		old := randBytes(rng.Intn(3), anyByte)
		new := randBytes(rng.Intn(4), anyByte)
		got := string(ps5019After(b, old, new))
		want := strings.ReplaceAll(string(b), string(old), string(new))
		if got != want {
			t.Fatalf("bytes/strings divergence on (%q, %q, %q): bytes=%q strings=%q", b, old, new, got, want)
		}
	}
}

// TestEquiv_PS5019_AllocParity pins that the rewrite is pure work
// removal: the Before and After allocate identically (exactly one
// result allocation on match AND on no-match alike — bytes.Replace
// always returns a fresh copy, unlike its strings twin), so the
// benchmark delta is entirely the redundant explicit Count scan.
func TestEquiv_PS5019_AllocParity(t *testing.T) {
	b := []byte("a payload that is comfortably long enough to heap-allocate on copy")
	var sink []byte
	for _, old := range [][]byte{[]byte("absent"), []byte("o")} {
		before := testing.AllocsPerRun(100, func() {
			sink = bytes.Replace(b, old, []byte("-"), bytes.Count(b, old))
		})
		after := testing.AllocsPerRun(100, func() {
			sink = bytes.ReplaceAll(b, old, []byte("-"))
		})
		if before != after {
			t.Fatalf("alloc profile diverged for old=%q: before %v, after %v — PS5019's pure-work-removal premise no longer holds", old, before, after)
		}
	}
	_ = sink
}
