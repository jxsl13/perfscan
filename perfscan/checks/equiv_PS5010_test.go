package checks

// Runtime differential for PS5010: string(bytes.ToUpper([]byte(s))) vs
// strings.ToUpper(s), plus the ToLower/ToTitle/Title siblings. The fix's
// safety argument is that each bytes function shares the identical
// machinery with its strings twin — the all-ASCII fast path, the same
// unicode case tables and special-casing, and rune decoding via
// utf8.DecodeRune (bytes) vs DecodeRuneInString (strings), which agree
// on every byte sequence: an invalid sequence yields RuneError width 1
// in both, and both spellings then re-encode that RuneError as the
// U+FFFD replacement bytes, so even the invalid-input mangling
// coincides byte for byte. The mapping is a pure per-rune
// transformation with no normalization and no context sensitivity, so
// the output string VALUE is identical for every s. This suite pins
// that claim over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 3 over an
//     adversarial alphabet (ASCII upper/lower, NUL, the bytes of ß and
//     ς and İ and the Kelvin sign so truncated and complete sequences
//     both arise, a bare continuation byte, an invalid lead byte, and
//     0xFF);
//   - targeted seeds (the classic Unicode case-mapping traps: German ß
//     whose upper is the TWO-rune SS, final sigma ς, Kelvin sign K and
//     ohm sign Ω which lowercase to plain k/ω, dotless ı and dotted İ,
//     the titlecase digraph ǅ, ligatures, long inputs on and off the
//     ASCII fast path, invalid UTF-8 at every position);
//   - randomized inputs over byte distributions biased toward ASCII,
//     multi-byte leads, bare continuations, and invalid bytes, with a
//     fixed seed.
//
// It also pins the perf premise: on all-ASCII no-change inputs the
// strings twins allocate NOTHING (they return s itself), which is the
// strongest form of the win.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// ps5010Before* are the exact Before-shapes of the check.
func ps5010BeforeUpper(s string) string { return string(bytes.ToUpper([]byte(s))) }
func ps5010BeforeLower(s string) string { return string(bytes.ToLower([]byte(s))) }
func ps5010BeforeTitle(s string) string { return string(bytes.ToTitle([]byte(s))) }

func ps5010BeforeWordTitle(s string) string {
	//lint:ignore SA1019 bytes.Title is deprecated, but it is exactly what PS5010 matches; its strings twin is equally deprecated, so the rewrite preserves the deprecation posture and the equivalence must be pinned.
	return string(bytes.Title([]byte(s)))
}

// ps5010After* are the exact After-shapes of the check.
func ps5010AfterUpper(s string) string { return strings.ToUpper(s) }
func ps5010AfterLower(s string) string { return strings.ToLower(s) }
func ps5010AfterTitle(s string) string { return strings.ToTitle(s) }

func ps5010AfterWordTitle(s string) string {
	//lint:ignore SA1019 see ps5010BeforeWordTitle.
	return strings.Title(s)
}

func ps5010Check(t *testing.T, s string) {
	t.Helper()
	if before, after := ps5010BeforeUpper(s), ps5010AfterUpper(s); before != after {
		t.Fatalf("ToUpper divergence on %q:\n before=%q\n after=%q", s, before, after)
	}
	if before, after := ps5010BeforeLower(s), ps5010AfterLower(s); before != after {
		t.Fatalf("ToLower divergence on %q:\n before=%q\n after=%q", s, before, after)
	}
	if before, after := ps5010BeforeTitle(s), ps5010AfterTitle(s); before != after {
		t.Fatalf("ToTitle divergence on %q:\n before=%q\n after=%q", s, before, after)
	}
	if before, after := ps5010BeforeWordTitle(s), ps5010AfterWordTitle(s); before != after {
		t.Fatalf("Title divergence on %q:\n before=%q\n after=%q", s, before, after)
	}
}

func TestEquiv_PS5010_ExhaustiveShort(t *testing.T) {
	alphabet := []byte{
		'a', 'Z', // ASCII fast-path members
		' ', '.', // word boundaries for Title
		0x00,       // NUL
		0xC3, 0x9F, // C3 9F = ß (U+00DF), whose upper is the two-rune "SS"
		0xCF, 0x82, // CF 82 = ς (U+03C2), final sigma
		0xC4, 0xB0, // C4 B0 = İ (U+0130), whose lower is the two-rune "i̇"
		0xE2, 0x84, 0xAA, // E2 84 AA = K (U+212A), the Kelvin sign
		0x80, // bare continuation byte
		0xFF, // never valid in UTF-8
	}
	var buf [3]byte
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			ps5010Check(t, string(buf[:n]))
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

func TestEquiv_PS5010_TargetedSeeds(t *testing.T) {
	seeds := []string{
		"",
		"hello world",
		"HELLO WORLD",
		"Mixed Case Input",
		"straße",                                // ß uppercases to the TWO-rune SS: the output is longer than the input
		"ẞSSß",                                  // capital sharp S vs the ss forms
		"ΌΣΟΣ όσος",                             // sigma in medial and final position
		"σςΣ",                                   // all three sigmas
		"K k K",                                 // Kelvin sign lowers to plain k; plain K stays K
		"Ω Ω ω",                                 // ohm sign lowers to plain ω
		"ı I i İ",                               // dotless/dotted i family (the non-Turkish tables)
		"ǄǅǆLJljǉ",                              // titlecase digraphs: upper, title and lower forms differ
		"ﬀ ﬁ ﬂ ﬅ",                               // ligatures: unchanged by ToUpper (no full case folding)
		"ᾀᾈᾷ",                                   // Greek with ypogegrammeni: title-vs-upper special forms
		"ⅷ Ⅷ ⅸ",                                 // Roman numeral compatibility characters
		"a\x00B",                                // NUL inside
		"\xff\xfe",                              // invalid UTF-8 only
		"\x80",                                  // bare continuation byte
		"a\x80b",                                // invalid byte between ASCII
		"\xc3",                                  // truncated ß prefix at the very end
		"\xc3(",                                 // invalid two-byte sequence
		"\xe2\x84",                              // truncated Kelvin-sign prefix
		"�",                                     // literal replacement char: maps to itself
		"word boundaries: o'neill mc-adams 3rd", // Title word-splitting shapes
		strings.Repeat("nochange", 4096),        // long all-ASCII no-change: the fast path both sides
		strings.Repeat("MiXeD", 4096),           // long all-ASCII with changes
		strings.Repeat("aß", 4096),              // long input growing under ToUpper
		strings.Repeat("Σσς ", 2048),            // long unicode path with final sigmas
		strings.Repeat("x\xffy", 2048),          // long input with interleaved invalid bytes
	}
	for _, s := range seeds {
		ps5010Check(t, s)
	}
}

func TestEquiv_PS5010_Randomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25010))
	pickers := []func() byte{
		func() byte { return byte(rng.Intn(128)) },       // ASCII
		func() byte { return byte(0xC0 + rng.Intn(32)) }, // two-byte leads (incl. invalid C0/C1)
		func() byte { return byte(0x80 + rng.Intn(64)) }, // bare continuations
		func() byte { return byte(0xE0 + rng.Intn(32)) }, // three/four-byte leads + invalid
		func() byte { return byte(rng.Intn(256)) },       // anything
		func() byte { // case-active bytes (sliced mid-rune on purpose)
			const active = "aA zZßςΣKİı"
			return active[rng.Intn(len(active))]
		},
	}
	for range 200000 {
		pick := pickers[rng.Intn(len(pickers))]
		b := make([]byte, rng.Intn(25))
		for i := range b {
			b[i] = pick()
		}
		ps5010Check(t, string(b))
	}
}

// TestEquiv_PS5010_FastPathIsZeroAlloc pins the strongest form of the
// perf premise: on an all-ASCII input that the mapping leaves unchanged,
// the strings twins return s itself and allocate NOTHING, where the
// Before-shape unconditionally pays the round-trip copies. The general
// Before/After costs are measured in benchmarks/ps5010_test.go.
func TestEquiv_PS5010_FastPathIsZeroAlloc(t *testing.T) {
	upper := "AN ALL-ASCII INPUT THAT IS ALREADY UPPERCASE AND HEAP-SIZED"
	lower := "an all-ascii input that is already lowercase and heap-sized"
	var sink string
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.ToUpper(upper)
		sink = strings.ToLower(lower)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the strings no-change fast path allocated %v times per run; the zero-alloc premise of PS5010 no longer holds", allocs)
	}
}
