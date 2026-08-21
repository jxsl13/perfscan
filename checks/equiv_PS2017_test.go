package checks

// Runtime differential for PS2017: string(bytes.Map(f, []byte(s))) vs
// strings.Map(f, s). The fix's safety argument is that the two Map
// implementations share the same contract executed the same way: decode
// s as UTF-8 rune by rune (utf8.DecodeRune in bytes vs the
// range-over-string decoder plus DecodeRuneInString in strings, which
// agree on every byte sequence — an invalid byte yields RuneError with
// width 1 in both), call the SAME mapping function f exactly once per
// rune position, in order, with the same rune arguments, drop runes for
// which f returns a negative value, and UTF-8-encode every non-negative
// result via the same utf8 machinery (out-of-range or surrogate results
// encode as RuneError in both). The subtle corners coincide too: an
// invalid input byte whose RuneError maps to itself is re-encoded as
// the three replacement-character bytes by BOTH spellings, and a
// literal U+FFFD in the input survives byte-for-byte in both. So the
// output string VALUE is identical for every s and every f. This suite
// pins that claim over:
//
//   - a SPECTRUM of mapping functions (identity, the unicode case
//     tables, rune-dropping including dropping RuneError itself,
//     ASCII-to-multibyte growth, RuneError-pinning, out-of-range and
//     surrogate results, and a plain shift);
//   - EXHAUSTIVE short inputs: every byte string of length <= 3 over an
//     adversarial alphabet (ASCII incl. mapping-active letters, NUL,
//     the bytes of ß and ς and İ and the Kelvin sign so truncated and
//     complete sequences both arise, the U+FFFD encoding bytes, a bare
//     continuation byte, and 0xFF);
//   - targeted seeds (case-mapping traps, literal U+FFFD, invalid UTF-8
//     at every position, long inputs on and off the no-change path);
//   - randomized inputs over byte distributions biased toward ASCII,
//     multi-byte leads, bare continuations, and invalid bytes, with a
//     fixed seed.
//
// It also pins the behavioral side conditions: f is called the same
// number of times with the same arguments in the same order (side
// effects preserved), a nil f behaves identically in both spellings,
// and on a no-change input strings.Map allocates NOTHING (it returns s
// itself), which is the strongest form of the win.

import (
	"bytes"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// ps2017Before and ps2017After are the exact Before/After shapes of the
// check.
func ps2017Before(f func(rune) rune, s string) string { return string(bytes.Map(f, []byte(s))) }
func ps2017After(f func(rune) rune, s string) string  { return strings.Map(f, s) }

// ps2017Mappings is the mapping-function spectrum: every branch of both
// implementations is reachable through it (unchanged runes, changed
// runes, dropped runes, RuneError produced from valid AND invalid
// inputs, growth past the input length, and invalid mapping results).
var ps2017Mappings = []struct {
	name string
	f    func(rune) rune
}{
	{"identity", func(r rune) rune { return r }},
	{"toUpper", unicode.ToUpper},
	{"toLower", unicode.ToLower},
	{"dropVowelsAndRuneError", func(r rune) rune {
		switch r {
		case 'a', 'e', 'i', 'o', 'u', utf8.RuneError:
			return -1
		}
		return r
	}},
	{"growAsciiToMultibyte", func(r rune) rune {
		switch r {
		case 'a':
			return 'ß'
		case 'z':
			return '\U0001D537' // 𝔷: a four-byte encoding
		}
		return r
	}},
	{"pinRuneError", func(rune) rune { return utf8.RuneError }},
	{"invalidResults", func(r rune) rune {
		switch r {
		case 'q':
			return utf8.MaxRune + 1 // out of range: encodes as RuneError in both
		case 'w':
			return 0xD800 // surrogate: encodes as RuneError in both
		}
		return r
	}},
	{"shiftByOne", func(r rune) rune { return r + 1 }},
}

func ps2017Check(t *testing.T, s string) {
	t.Helper()
	for _, m := range ps2017Mappings {
		if before, after := ps2017Before(m.f, s), ps2017After(m.f, s); before != after {
			t.Fatalf("Map(%s) divergence on %q:\n before=%q\n after=%q", m.name, s, before, after)
		}
	}
}

func TestEquiv_PS2017_ExhaustiveShort(t *testing.T) {
	alphabet := []byte{
		'a', 'q', 'Z', // ASCII: mapping-active and plain
		0x00,       // NUL
		0xC3, 0x9F, // C3 9F = ß (U+00DF): upper is the two-rune "SS"
		0xCF, 0x82, // CF 82 = ς (U+03C2): final sigma
		0xC4, 0xB0, // C4 B0 = İ (U+0130): lower is the two-rune "i̇"
		0xE2, 0x84, 0xAA, // E2 84 AA = K (U+212A): the Kelvin sign
		0xEF, 0xBF, 0xBD, // EF BF BD = U+FFFD itself: the RuneError corner
		0x80, // bare continuation byte
		0xFF, // never valid in UTF-8
	}
	var buf [3]byte
	var rec func(n, depth int)
	rec = func(n, depth int) {
		if depth == n {
			ps2017Check(t, string(buf[:n]))
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

func TestEquiv_PS2017_TargetedSeeds(t *testing.T) {
	seeds := []string{
		"",
		"hello world",
		"HELLO WORLD",
		"Mixed Case Input",
		"straße",                         // ß uppercases to the TWO-rune SS: growth
		"aeiou AEIOU y",                  // vowels: fully dropped by the dropping mapping
		"σςΣ",                            // all three sigmas
		"K k Ω ω",                        // Kelvin and ohm signs lower to plain k/ω
		"ı I i İ",                        // dotless/dotted i family
		"qwqw",                           // invalid mapping results at every position
		"azaz",                           // growth mapping active at every position
		"a\x00B",                         // NUL inside
		"\xff\xfe",                       // invalid UTF-8 only
		"\x80",                           // bare continuation byte
		"a\x80b",                         // invalid byte between ASCII
		"\xc3",                           // truncated ß prefix at the very end
		"\xc3(",                          // invalid two-byte sequence
		"\xe2\x84",                       // truncated Kelvin-sign prefix
		"\xed\xa0\x80",                   // an encoded surrogate: RuneError per byte in both
		"\xc0\xaf",                       // overlong encoding: rejected byte-by-byte in both
		"�",                              // literal replacement char: kept when unchanged, in both
		"a�b\xffc",                       // literal U+FFFD next to a real invalid byte
		strings.Repeat("nochange", 4096), // long no-change input: the return-s path
		strings.Repeat("MiXeD", 4096),    // long input changing under the case tables
		strings.Repeat("aß", 4096),       // long input growing under ToUpper
		strings.Repeat("Σσς ", 2048),     // long unicode path with final sigmas
		strings.Repeat("x\xffy", 2048),   // long input with interleaved invalid bytes
	}
	for _, s := range seeds {
		ps2017Check(t, s)
	}
}

func TestEquiv_PS2017_Randomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25017))
	pickers := []func() byte{
		func() byte { return byte(rng.Intn(128)) },       // ASCII
		func() byte { return byte(0xC0 + rng.Intn(32)) }, // two-byte leads (incl. invalid C0/C1)
		func() byte { return byte(0x80 + rng.Intn(64)) }, // bare continuations
		func() byte { return byte(0xE0 + rng.Intn(32)) }, // three/four-byte leads + invalid
		func() byte { return byte(rng.Intn(256)) },       // anything
		func() byte { // mapping-active bytes (sliced mid-rune on purpose)
			const active = "aeqwzA ZßςΣKİı"
			return active[rng.Intn(len(active))]
		},
	}
	for range 60000 {
		pick := pickers[rng.Intn(len(pickers))]
		b := make([]byte, rng.Intn(25))
		for i := range b {
			b[i] = pick()
		}
		ps2017Check(t, string(b))
	}
}

// TestEquiv_PS2017_CallSequence pins the side-effect contract: f is
// called the same number of times, with the same rune arguments, in the
// same order, in both spellings — so a mapping function with side
// effects behaves identically after the rewrite.
func TestEquiv_PS2017_CallSequence(t *testing.T) {
	seeds := []string{
		"",
		"plain ascii",
		"Mixed straße ΣςK",
		"a\x80b\xff\xc3(",
		"�x�",
		"aeiou qw az",
	}
	for _, s := range seeds {
		var beforeCalls, afterCalls []rune
		before := string(bytes.Map(func(r rune) rune {
			beforeCalls = append(beforeCalls, r)
			return unicode.ToUpper(r)
		}, []byte(s)))
		after := strings.Map(func(r rune) rune {
			afterCalls = append(afterCalls, r)
			return unicode.ToUpper(r)
		}, s)
		if before != after {
			t.Fatalf("output divergence on %q: before=%q after=%q", s, before, after)
		}
		if !slices.Equal(beforeCalls, afterCalls) {
			t.Fatalf("call-sequence divergence on %q:\n before=%v\n after=%v", s, beforeCalls, afterCalls)
		}
	}
}

// TestEquiv_PS2017_NilMapping pins the degenerate f: a nil mapping
// behaves identically in both spellings — the empty input returns ""
// without ever calling f, and a non-empty input panics (calling a nil
// function) in both.
func TestEquiv_PS2017_NilMapping(t *testing.T) {
	if before, after := ps2017Before(nil, ""), ps2017After(nil, ""); before != "" || after != "" {
		t.Fatalf("nil mapping on empty input: before=%q after=%q, want both empty", before, after)
	}
	panics := func(g func()) (panicked bool) {
		defer func() { panicked = recover() != nil }()
		g()
		return false
	}
	if b, a := panics(func() { ps2017Before(nil, "x") }), panics(func() { ps2017After(nil, "x") }); !b || !a {
		t.Fatalf("nil mapping on non-empty input: before panicked=%v, after panicked=%v, want both true", b, a)
	}
}

// TestEquiv_PS2017_NoChangeIsZeroAlloc pins the strongest form of the
// perf premise: when f changes nothing, strings.Map returns s itself
// and allocates NOTHING, where the Before-shape unconditionally pays
// the round-trip copies. The general Before/After costs are measured in
// benchmarks/ps2017_test.go.
func TestEquiv_PS2017_NoChangeIsZeroAlloc(t *testing.T) {
	s := strings.Repeat("an input the identity mapping leaves unchanged ", 4)
	ident := func(r rune) rune { return r }
	var sink string
	allocs := testing.AllocsPerRun(100, func() {
		sink = strings.Map(ident, s)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("the strings.Map no-change path allocated %v times per run; the zero-alloc premise of PS2017 no longer holds", allocs)
	}
}
