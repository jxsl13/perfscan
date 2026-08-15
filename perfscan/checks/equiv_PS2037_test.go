package checks

// Runtime differential for PS2037: string([]rune{r}) versus
// string(rune(r)). The fix's safety argument is that a []rune composite
// literal converts its single element to rune exactly as the explicit
// rune(E) conversion does, and the []rune->string conversion over a
// one-element slice runs the identical UTF-8 encoder on that one rune as
// the direct rune->string conversion — including every INVALID rune
// (negative, above utf8.MaxRune, the surrogate range), which both forms
// encode as the U+FFFD replacement bytes. This suite pins:
//
//   - byte identity over an exhaustive sweep of the entire BMP boundary
//     neighborhoods, the full surrogate range, all UTF-8 width
//     boundaries, int32 extremes, and millions of seeded-random int32
//     values (positive and negative);
//   - the untyped-constant element shapes the check accepts
//     ([]rune{65}, []rune{'日'}) against their rewrites;
//   - evaluation count and order parity: the element expression runs
//     exactly once in both spellings;
//   - the divergence the single-positional-element gate excludes,
//     reproduced positively: the keyed literal []rune{5: r} pads with
//     zero runes and is NOT the single-rune encoding;
//   - the perf premise: both forms are pure functions of r, so identity
//     over the probe set is identity of the whole rewrite.

import (
	"math"
	"math/rand"
	"testing"
	"unicode/utf8"
)

//go:noinline
func ps2037Before(r rune) string { return string([]rune{r}) }

//go:noinline
func ps2037After(r rune) string { return string(rune(r)) }

func TestEquiv_PS2037_ByteIdentity(t *testing.T) {
	probe := func(r rune) {
		t.Helper()
		if b, a := ps2037Before(r), ps2037After(r); b != a {
			t.Fatalf("string([]rune{%d}) diverges: before=%q after=%q", r, b, a)
		}
	}
	// Exhaustive over every UTF-8 width boundary neighborhood, the FULL
	// surrogate range, the MaxRune frontier and a negative band.
	for r := rune(-4096); r <= 0x11000; r++ {
		probe(r)
	}
	for r := rune(0xD7FF - 16); r <= 0xE000+16; r++ {
		probe(r)
	}
	for _, r := range []rune{
		math.MinInt32, math.MaxInt32, -1, 0, 1,
		0x7F, 0x80, 0x7FF, 0x800, 0xFFFF, 0x10000,
		utf8.MaxRune, utf8.MaxRune + 1, utf8.RuneError,
		0xD800, 0xDBFF, 0xDC00, 0xDFFF, // surrogates -> U+FFFD in both
		0x1F600,
	} {
		probe(r)
	}
	rng := rand.New(rand.NewSource(0x2037))
	for range 2_000_000 {
		probe(rune(rng.Uint32())) // full int32 range incl. negatives
	}
}

// The untyped-constant element shapes the check accepts: the constant
// takes the element type from context in the Before and converts
// explicitly in the After — same rune value, same bytes.
func TestEquiv_PS2037_UntypedConstants(t *testing.T) {
	if string([]rune{65}) != string(rune(65)) {
		t.Fatal("[]rune{65} diverges from rune(65)")
	}
	if string([]rune{'日'}) != string(rune('日')) {
		t.Fatal("[]rune{'日'} diverges from rune('日')")
	}
	if string([]rune{utf8.MaxRune + 1}) != string(rune(utf8.MaxRune+1)) {
		t.Fatal("invalid constant rune diverges")
	}
}

// Evaluation count and order parity: the element expression is evaluated
// exactly once in both spellings.
func TestEquiv_PS2037_EvaluationParity(t *testing.T) {
	calls := 0
	next := func() rune { calls++; return '☃' }
	if got := string([]rune{next()}); got != "☃" || calls != 1 {
		t.Fatalf("before: got %q after %d calls", got, calls)
	}
	calls = 0
	if got := string(rune(next())); got != "☃" || calls != 1 {
		t.Fatalf("after: got %q after %d calls", got, calls)
	}
}

// The divergence the keyed-element gate excludes, reproduced positively:
// []rune{5: r} pads the slice with five zero runes, so its string is six
// runes — NOT string(rune(r)). The check only ever fires on a single
// POSITIONAL element.
func TestEquiv_PS2037_KeyedLiteralDiverges(t *testing.T) {
	r := rune('x')
	padded := string([]rune{5: r})
	direct := string(rune(r))
	if padded == direct {
		t.Fatal("expected []rune{5: r} to diverge from rune(r) — the keyed gate would be unnecessary")
	}
	if utf8.RuneCountInString(padded) != 6 || utf8.RuneCountInString(direct) != 1 {
		t.Fatalf("unexpected rune counts: padded=%d direct=%d", utf8.RuneCountInString(padded), utf8.RuneCountInString(direct))
	}
}
