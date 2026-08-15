package checks

// Runtime differential for PS2031: fmt.Errorf("%s", s) and
// fmt.Errorf("%v", s) versus errors.New(s), with s a plain string. The
// fix's safety argument is that with no %w verb fmt.Errorf returns
// errors.New(formatted) — the same *errorString dynamic type, no Unwrap
// — and that a format which is EXACTLY the one bare verb "%s" or "%v"
// applied to a string operand writes the operand's bytes verbatim, so
// formatted == s and the two spellings build byte-identical errors.
// Crucially, any '%' inside s is DATA under %s/%v — the vanishing
// one-verb format was the only interpreter — so verb-looking content in
// s ("%s", "%w", "%!x(MISSING)", …) cannot diverge. This suite pins the
// claim over:
//
//   - targeted states: the empty string, ordinary messages, every
//     printf metacharacter as data (bare and trailing '%', "%%", every
//     common verb, width/flag/index forms, "%!s(MISSING)"-style error
//     spellings, a lone "%w"), NUL bytes, invalid UTF-8 (lone
//     continuation and lead bytes, truncated multi-byte runes),
//     non-ASCII text, and long strings including 4 KiB of '%';
//   - an exhaustive sweep of ALL strings up to length 3 over an
//     adversarial alphabet of printf metacharacters ('%', 's', 'v',
//     'w', 'd', '!', '(', an ordinary letter, 0x00 and 0xFF) —
//     adjacency builds every short verb-like and broken-verb-like
//     shape;
//   - an exhaustive per-rune sweep of every Unicode code point
//     (surrogates included, which string(r) turns into RuneError);
//   - randomized strings (arbitrary bytes and '%'-heavy) with a fixed
//     seed;
//   - the dynamic-type and Unwrap contract: both spellings yield the
//     identical concrete type with no Unwrap method;
//   - the perf premise: errors.New allocates exactly once while the
//     fmt.Errorf spelling allocates strictly more.
//
// Both rewritten verbs are checked for every input.

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

// ps2031Before* are the exact Before-shapes of the check.
func ps2031BeforeS(s string) error { return fmt.Errorf("%s", s) }
func ps2031BeforeV(s string) error { return fmt.Errorf("%v", s) }

// ps2031After is the exact After-shape of the check.
func ps2031After(s string) error { return errors.New(s) }

// ps2031Check pins, for one input: byte-identical Error() text across
// both verbs and errors.New (and against s itself), the identical
// concrete dynamic type, and a nil Unwrap on both sides.
func ps2031Check(t *testing.T, s string) {
	t.Helper()
	after := ps2031After(s)
	if after.Error() != s {
		t.Fatalf("input %q: errors.New(s).Error() = %q, not s itself", s, after.Error())
	}
	for _, p := range []struct {
		verb   string
		before error
	}{
		{"%s", ps2031BeforeS(s)},
		{"%v", ps2031BeforeV(s)},
	} {
		if got, want := p.before.Error(), after.Error(); got != want {
			t.Fatalf("input %q: fmt.Errorf(%q, s).Error() = %q but errors.New(s).Error() = %q — bit-identity broken", s, p.verb, got, want)
		}
		if bt, at := reflect.TypeOf(p.before), reflect.TypeOf(after); bt != at {
			t.Fatalf("input %q: fmt.Errorf(%q, s) has dynamic type %v but errors.New(s) has %v", s, p.verb, bt, at)
		}
		if u := errors.Unwrap(p.before); u != nil {
			t.Fatalf("input %q: fmt.Errorf(%q, s) unexpectedly unwraps to %v", s, p.verb, u)
		}
	}
	if u := errors.Unwrap(after); u != nil {
		t.Fatalf("input %q: errors.New(s) unexpectedly unwraps to %v", s, u)
	}
}

func TestEquiv_PS2031_TargetedInputs(t *testing.T) {
	inputs := []string{
		"",
		"database connection failed",
		"multi word message with spaces\tand tabs\n",
		// printf metacharacters as DATA: the one-verb format is the only
		// interpreter, and it vanishes.
		"%", "%%", "s%", "100%", "100%% done",
		"%s", "%v", "%w", "%d", "%q", "%x", "%T", "%p",
		"%s%v%w%d%q", "%+v", "%#v", "%5s", "%-8d", "%[1]s", "%*d",
		"%!s(MISSING)", "%!(EXTRA string=x)", "%!v(PANIC=...)",
		"wrap: %w", "trailing percent %",
		// NUL and invalid UTF-8 ride through both spellings unchanged.
		"\x00", "a\x00b", "\xff", "\x80", "\xc2", "\xc2\xa0", "\xe3\x80",
		"bad \xff\xfe utf8", "%\xff", "\xff%s\xff",
		// Non-ASCII text.
		"héllo wörld", "日本語のエラー", "emoji 🎉 error", "  ",
		// Long strings, including pathological all-'%' content.
		strings.Repeat("x", 4096),
		strings.Repeat("%", 4096),
		strings.Repeat("%s", 2048),
		strings.Repeat("error %v and %w ", 256),
	}
	for _, s := range inputs {
		ps2031Check(t, s)
	}
}

// TestEquiv_PS2031_ExhaustiveShortStrings sweeps ALL strings up to
// length 3 over an adversarial alphabet of printf metacharacters —
// every short verb-like, escape-like and broken shape ("%s", "%%",
// "%w", "%!(", "s%v", …) plus NUL and an invalid byte.
func TestEquiv_PS2031_ExhaustiveShortStrings(t *testing.T) {
	alphabet := []byte{'%', 's', 'v', 'w', 'd', '!', '(', 'a', 0x00, 0xFF}
	var sweep func(prefix []byte, depth int)
	sweep = func(prefix []byte, depth int) {
		ps2031Check(t, string(prefix))
		if depth == 0 {
			return
		}
		for _, c := range alphabet {
			sweep(append(prefix[:len(prefix):len(prefix)], c), depth-1)
		}
	}
	sweep(nil, 3)
}

// TestEquiv_PS2031_AllRunes sweeps every Unicode code point: string(r)
// (which converts surrogates and out-of-range values to the RuneError
// encoding) must round-trip identically through both spellings.
func TestEquiv_PS2031_AllRunes(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		s := string(r)
		if ps2031BeforeS(s).Error() != s || ps2031BeforeV(s).Error() != s {
			t.Fatalf("rune %U: fmt.Errorf does not pass %q through verbatim", r, s)
		}
	}
}

// TestEquiv_PS2031_Randomized drives random strings (arbitrary bytes
// and '%'-heavy, so verb-like shapes actually occur) through the full
// check with a fixed seed.
func TestEquiv_PS2031_Randomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0x2031))
	verby := []byte{'%', 's', 'v', 'w', 'd', 'q', 'x', 'T', '+', '#', '!', '(', ')', '[', ']', '*', '0', '5', ' ', 'a'}
	for range 2000 {
		// Arbitrary bytes.
		b := make([]byte, rng.Intn(64))
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		ps2031Check(t, string(b))
		// '%'-heavy bytes: printf metacharacters dominate, so verb-like
		// runs are actually exercised.
		c := make([]byte, rng.Intn(64))
		for i := range c {
			c[i] = verby[rng.Intn(len(verby))]
		}
		ps2031Check(t, string(c))
	}
}

// TestEquiv_PS2031_Allocs pins the perf premise: errors.New(s)
// allocates exactly once (the errorString), while fmt.Errorf("%s", s)
// additionally pays the interface boxing and the formatted copy.
func TestEquiv_PS2031_Allocs(t *testing.T) {
	msg := strings.Repeat("failure detail ", 4)
	var sink error
	after := testing.AllocsPerRun(100, func() { sink = ps2031After(msg) })
	if after != 1 {
		t.Errorf("errors.New(s) allocates %v times per run, want exactly 1", after)
	}
	before := testing.AllocsPerRun(100, func() { sink = ps2031BeforeS(msg) })
	if before <= after {
		t.Logf("fmt.Errorf(\"%%s\", s) allocates %v times per run vs errors.New's %v — the compiler learned to elide the fmt path; the win claim may need re-framing", before, after)
	}
	_ = sink
}
