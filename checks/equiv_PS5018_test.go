package checks

// Runtime differential for PS5018: strings.Map(unicode.ToUpper, s) vs
// strings.ToUpper(s), and the ToLower twin. The fix's safety argument
// is two-sided. Non-ASCII path: strings.ToUpper/ToLower are DEFINED as
// `return Map(unicode.ToUpper/ToLower, s)` — textually the very call
// the user wrote, so invalid/truncated UTF-8 and every case trap run
// the shared code. All-ASCII path: ranging over a byte below 0x80
// yields that byte as a width-1 rune, and unicode.ToUpper/ToLower over
// ASCII changes only 'a'-'z' <-> 'A'-'Z' (never negative, so Map drops
// nothing) — exactly what the fast-path byte loop computes. Strings
// are immutable, so value (byte-content) equality is the WHOLE
// observable story — there is no capacity or mutate-the-result
// dimension; both spellings even return s itself when nothing changes
// (Map's unchanged-input fast path and ToUpper/ToLower's
// !hasLower/!hasUpper fast path both end in `return s`). This suite
// pins string equality over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 3 over an
//     adversarial alphabet (ASCII upper/lower, NUL, the bytes of ß and
//     ς and İ and the Kelvin sign so truncated and complete sequences
//     both arise, a bare continuation byte, an invalid lead byte, and
//     0xFF);
//   - targeted seeds (ß which has no simple uppercase, final sigma ς,
//     Kelvin sign and ohm sign, dotless ı and dotted İ, titlecase
//     digraphs, invalid UTF-8 at every position, long inputs on and off
//     the ASCII fast path);
//   - randomized inputs over byte distributions biased toward ASCII,
//     multi-byte leads, bare continuations, and invalid bytes, with a
//     fixed seed.

import (
	"math/rand"
	"strings"
	"testing"
	"unicode"
)

// ps5018Before* are the exact Before-shapes of the check.
func ps5018BeforeUpper(s string) string { return strings.Map(unicode.ToUpper, s) }
func ps5018BeforeLower(s string) string { return strings.Map(unicode.ToLower, s) }

// ps5018After* are the exact After-shapes of the check.
func ps5018AfterUpper(s string) string { return strings.ToUpper(s) }
func ps5018AfterLower(s string) string { return strings.ToLower(s) }

func ps5018Check(t *testing.T, s string) {
	t.Helper()
	if before, after := ps5018BeforeUpper(s), ps5018AfterUpper(s); before != after {
		t.Fatalf("ToUpper divergence on %q:\n before=%q\n after=%q", s, before, after)
	}
	if before, after := ps5018BeforeLower(s), ps5018AfterLower(s); before != after {
		t.Fatalf("ToLower divergence on %q:\n before=%q\n after=%q", s, before, after)
	}
}

func TestEquiv_PS5018_ExhaustiveShort(t *testing.T) {
	alphabet := []byte{
		'a', 'Z', // ASCII fast-path members, one per casing direction
		0x00,       // NUL
		0xC3, 0x9F, // C3 9F = ß (U+00DF): no simple uppercase mapping, stays ß on both sides
		0xCF, 0x82, // CF 82 = ς (U+03C2), final sigma
		0xC4, 0xB0, // C4 B0 = İ (U+0130), the dotted capital I
		0xE2, 0x84, 0xAA, // E2 84 AA = K (U+212A), the Kelvin sign
		0x80, // bare continuation byte
		0xFF, // never valid in UTF-8
	}
	var buf [3]byte
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			ps5018Check(t, string(buf[:n]))
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

func TestEquiv_PS5018_TargetedSeeds(t *testing.T) {
	seeds := []string{
		"",
		"hello world",
		"HELLO WORLD",
		"Mixed Case Input",
		"straße",                         // ß: both spellings apply the same simple 1:1 unicode map
		"ẞSSß",                           // capital sharp S vs the ss forms
		"ΌΣΟΣ όσος",                      // sigma in medial and final position
		"σςΣ",                            // all three sigmas
		"K k K",                          // Kelvin sign lowers to plain k; plain K stays K
		"Ω Ω ω",                          // ohm sign lowers to plain ω
		"ı I i İ",                        // dotless/dotted i family (the non-Turkish tables)
		"ǄǅǆLJljǉ",                       // titlecase digraphs: upper and lower forms differ
		"ﬀ ﬁ ﬂ ﬅ",                        // ligatures: unchanged (no full case folding)
		"ᾀᾈᾷ",                            // Greek with ypogegrammeni
		"a\x00B",                         // NUL inside
		"\xff\xfe",                       // invalid UTF-8 only
		"\x80",                           // bare continuation byte
		"a\x80b",                         // invalid byte between ASCII
		"\xc3",                           // truncated ß prefix at the very end
		"\xc3(",                          // invalid two-byte sequence
		"\xe2\x84",                       // truncated Kelvin-sign prefix
		"�",                              // literal replacement char: maps to itself
		strings.Repeat("nochange", 4096), // long all-ASCII no-change: both return s itself
		strings.Repeat("MiXeD", 4096),    // long all-ASCII with changes: the tight byte loop
		strings.Repeat("aß", 4096),       // long non-ASCII input (the shared Map path)
		strings.Repeat("Σσς ", 2048),     // long unicode path with final sigmas
		strings.Repeat("x\xffy", 2048),   // long input with interleaved invalid bytes
	}
	for _, s := range seeds {
		ps5018Check(t, s)
	}
}

func TestEquiv_PS5018_Randomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25018))
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
		ps5018Check(t, string(b))
	}
}
