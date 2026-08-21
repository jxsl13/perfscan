package checks

// Runtime differential for PS5017: bytes.Map(unicode.ToUpper, b) vs
// bytes.ToUpper(b), and the ToLower twin. The fix's safety argument is
// two-sided. Non-ASCII path: bytes.ToUpper/ToLower are DEFINED as
// `return Map(unicode.ToUpper/ToLower, s)` — textually the very call
// the user wrote, so invalid/truncated UTF-8 and every case trap run
// the shared code. All-ASCII path: utf8.DecodeRune of a byte below
// 0x80 yields that byte as a width-1 rune, and unicode.ToUpper/ToLower
// over ASCII changes only 'a'-'z' <-> 'A'-'Z' (never negative, so Map
// drops nothing) — exactly what the fast-path byte loop computes. Both
// spellings always return a freshly allocated, never-input-aliasing
// slice, and both return a NON-nil empty slice for empty (or nil)
// input. The one accepted divergence is the result's CAPACITY (Map
// sizes to exactly len(b); the fast paths may round up to an allocator
// size class) — the PS2112 class: an unspecified implementation detail
// of a freshly built slice. This suite pins content equality, nil-ness
// equality, and both-sided non-aliasing over:
//
//   - EXHAUSTIVE short inputs: every byte string of length <= 3 over an
//     adversarial alphabet (ASCII upper/lower, NUL, the bytes of ß and
//     ς and İ and the Kelvin sign so truncated and complete sequences
//     both arise, a bare continuation byte, an invalid lead byte, and
//     0xFF);
//   - targeted seeds (ß whose upper is the TWO-rune SS, final sigma ς,
//     Kelvin sign and ohm sign, dotless ı and dotted İ, titlecase
//     digraphs, invalid UTF-8 at every position, long inputs on and off
//     the ASCII fast path);
//   - randomized inputs over byte distributions biased toward ASCII,
//     multi-byte leads, bare continuations, and invalid bytes, with a
//     fixed seed.

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unicode"
)

// ps5017Before* are the exact Before-shapes of the check.
func ps5017BeforeUpper(b []byte) []byte { return bytes.Map(unicode.ToUpper, b) }
func ps5017BeforeLower(b []byte) []byte { return bytes.Map(unicode.ToLower, b) }

// ps5017After* are the exact After-shapes of the check.
func ps5017AfterUpper(b []byte) []byte { return bytes.ToUpper(b) }
func ps5017AfterLower(b []byte) []byte { return bytes.ToLower(b) }

func ps5017Check(t *testing.T, b []byte) {
	t.Helper()
	if before, after := ps5017BeforeUpper(b), ps5017AfterUpper(b); !bytes.Equal(before, after) {
		t.Fatalf("ToUpper content divergence on %q:\n before=%q\n after=%q", b, before, after)
	} else if (before == nil) != (after == nil) {
		t.Fatalf("ToUpper nil-ness divergence on %q: before==nil %v, after==nil %v", b, before == nil, after == nil)
	}
	if before, after := ps5017BeforeLower(b), ps5017AfterLower(b); !bytes.Equal(before, after) {
		t.Fatalf("ToLower content divergence on %q:\n before=%q\n after=%q", b, before, after)
	} else if (before == nil) != (after == nil) {
		t.Fatalf("ToLower nil-ness divergence on %q: before==nil %v, after==nil %v", b, before == nil, after == nil)
	}
}

func TestEquiv_PS5017_ExhaustiveShort(t *testing.T) {
	alphabet := []byte{
		'a', 'Z', // ASCII fast-path members, one per casing direction
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
			ps5017Check(t, buf[:n])
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

func TestEquiv_PS5017_TargetedSeeds(t *testing.T) {
	seeds := []string{
		"",
		"hello world",
		"HELLO WORLD",
		"Mixed Case Input",
		"straße",                         // ß uppercases to the TWO-rune SS: the output is longer than the input
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
		strings.Repeat("nochange", 4096), // long all-ASCII no-change: the memmove copy path
		strings.Repeat("MiXeD", 4096),    // long all-ASCII with changes: the tight byte loop
		strings.Repeat("aß", 4096),       // long input growing under ToUpper (the shared Map path)
		strings.Repeat("Σσς ", 2048),     // long unicode path with final sigmas
		strings.Repeat("x\xffy", 2048),   // long input with interleaved invalid bytes
	}
	for _, s := range seeds {
		ps5017Check(t, []byte(s))
	}
}

func TestEquiv_PS5017_Randomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25017))
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
		ps5017Check(t, b)
	}
}

// TestEquiv_PS5017_NoInputAliasing pins the mutate-the-result premise:
// BOTH spellings return a freshly allocated slice, so writing into the
// result never changes the input (and vice versa) in either form —
// there is no strings-style return-s-itself hazard for the rewrite to
// introduce or remove.
func TestEquiv_PS5017_NoInputAliasing(t *testing.T) {
	for _, spell := range []struct {
		name string
		f    func([]byte) []byte
	}{
		{"Before/Map-ToUpper", ps5017BeforeUpper},
		{"After/ToUpper", ps5017AfterUpper},
		{"Before/Map-ToLower", ps5017BeforeLower},
		{"After/ToLower", ps5017AfterLower},
	} {
		for _, seed := range []string{"already upper NOT", "ALREADY LOWER not", "nichts zu tun", "größe MATTERS"} {
			in := []byte(seed)
			orig := append([]byte(nil), in...)
			out := spell.f(in)
			for i := range out {
				out[i] = '#'
			}
			if !bytes.Equal(in, orig) {
				t.Fatalf("%s aliases its input on %q: input mutated to %q", spell.name, orig, in)
			}
		}
	}
}
