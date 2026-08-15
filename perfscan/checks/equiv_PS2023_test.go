package checks

// Runtime differential for PS2023: fmt.Errorf("%s", s) and
// fmt.Errorf("%v", s) vs errors.New(s) for s a plain string. The safety
// argument: for a format with no %w wrap verb, fmt.Errorf returns
// errors.New(formatted) — and for a format of exactly "%s" or "%v" with
// a string operand, the formatted string is the operand verbatim (%s and
// %v write a plain string byte-for-byte, with no quoting, width or
// escaping). So both forms produce a *errors.errorString wrapping the
// SAME bytes: same dynamic type, same Error() output, no Unwrap on
// either side, and a fresh pointer from every call. Any '%' in s is
// DATA to %s/%v — copied, never re-parsed — so percent-bearing, empty,
// NUL-bearing and invalid-UTF-8 strings are all covered. The suite:
//
//   - EXHAUSTIVE short inputs: every string of length <= 4 over an
//     adversarial alphabet ('a', '%', 's', 'w', NUL, a lone 0xC3
//     continuation-truncating byte, and 0xFF which is never valid
//     UTF-8) — this enumerates every short smuggled verb ("%s", "%w",
//     "%%", "%%s", …) as argument DATA;
//   - targeted seeds: empty, bare and doubled percents, every common
//     verb spelled inside the data, fmt's own %! error markers,
//     embedded NUL, invalid UTF-8, multi-byte runes, newlines and a
//     long string;
//   - randomized long inputs over the full byte range with a fixed
//     seed;
//   - dynamic type, Unwrap absence, errors.Is/As-relevant identity:
//     both sides return a fresh, distinct error value per call;
//   - side-effect order: the operand is evaluated exactly once in both
//     forms.

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// ps2023BeforeS is the matched %s shape, verbatim.
func ps2023BeforeS(s string) error { return fmt.Errorf("%s", s) }

// ps2023BeforeV is the matched %v shape, verbatim.
func ps2023BeforeV(s string) error { return fmt.Errorf("%v", s) }

// ps2023After is the rewrite.
func ps2023After(s string) error { return errors.New(s) }

func ps2023Check(t *testing.T, s string) {
	t.Helper()
	after := ps2023After(s)
	for _, bf := range []struct {
		verb   string
		before error
	}{
		{"%s", ps2023BeforeS(s)},
		{"%v", ps2023BeforeV(s)},
	} {
		if got, want := bf.before.Error(), after.Error(); got != want {
			t.Fatalf("divergence on s=%q:\n fmt.Errorf(%q, s).Error() = %q\n errors.New(s).Error() = %q",
				s, bf.verb, got, want)
		}
		if bt, at := reflect.TypeOf(bf.before), reflect.TypeOf(after); bt != at {
			t.Fatalf("dynamic type divergence on s=%q: fmt.Errorf(%q, s) is %v, errors.New(s) is %v",
				s, bf.verb, bt, at)
		}
		if errors.Unwrap(bf.before) != nil {
			t.Fatalf("fmt.Errorf(%q, s) unexpectedly wraps for s=%q", bf.verb, s)
		}
		if errors.Unwrap(after) != nil {
			t.Fatalf("errors.New(s) unexpectedly wraps for s=%q", s)
		}
	}
}

// ps2023Alphabet anchors a plain ASCII byte, the percent sign plus the
// two verb letters so every short smuggled verb arises as data, NUL, a
// lone UTF-8 lead byte (truncated sequence) and 0xFF (never valid
// UTF-8).
var ps2023Alphabet = []byte{'a', '%', 's', 'w', 0x00, 0xC3, 0xFF}

func TestEquiv_PS2023_ExhaustiveShort(t *testing.T) {
	const maxLen = 4
	var enum func(buf []byte, depth int, emit func([]byte))
	enum = func(buf []byte, depth int, emit func([]byte)) {
		if depth == len(buf) {
			emit(buf)
			return
		}
		for _, c := range ps2023Alphabet {
			buf[depth] = c
			enum(buf, depth+1, emit)
		}
	}
	for n := 0; n <= maxLen; n++ {
		enum(make([]byte, n), 0, func(b []byte) {
			ps2023Check(t, string(b))
		})
	}
}

func TestEquiv_PS2023_TargetedSeeds(t *testing.T) {
	seeds := []string{
		"",                                  // empty message
		"%",                                 // lone percent, data
		"%%",                                // doubled percent, data
		"%s", "%v", "%w", "%d", "%q", "%+v", // smuggled verbs, data
		"%!s(MISSING)", "%!(EXTRA int=1)", // fmt's own error markers, data
		"100%% done",                          // percent escape spelled in data
		"a\x00b",                              // embedded NUL
		"h\xc3llo",                            // truncated rune
		"\xff\xfe",                            // invalid UTF-8
		"héllo wörld",                         // valid multi-byte runes
		"日本語",                                 // wide runes
		"line one\nline two\ttabbed",          // whitespace controls
		strings.Repeat("alpha %s beta ", 512), // long, verb-riddled
	}
	for _, s := range seeds {
		ps2023Check(t, s)
	}
}

func TestEquiv_PS2023_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25082023))
	for range 50000 {
		b := make([]byte, rng.Intn(128))
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		ps2023Check(t, string(b))
	}
}

// TestEquiv_PS2023_FreshIdentity pins that the == identity class is
// preserved: BOTH forms return a fresh, distinct error value on every
// call (fmt.Errorf builds a new *errorString via errors.New internally;
// errors.New returns a pointer to a new allocation), so no program can
// observe an identity change — two befores are unequal exactly as two
// afters are, and each error errors.Is-matches only itself.
func TestEquiv_PS2023_FreshIdentity(t *testing.T) {
	const s = "same message"
	if ps2023BeforeS(s) == ps2023BeforeS(s) {
		t.Fatal("two before calls (fmt.Errorf with the one-verb format) unexpectedly compare equal")
	}
	if ps2023After(s) == ps2023After(s) {
		t.Fatal("two errors.New(s) calls unexpectedly compare equal")
	}
	b, a := ps2023BeforeS(s), ps2023After(s)
	if errors.Is(b, a) || errors.Is(a, b) {
		t.Fatal("distinct error values unexpectedly errors.Is-match")
	}
	if !errors.Is(b, b) || !errors.Is(a, a) {
		t.Fatal("an error value must errors.Is-match itself")
	}
}

// TestEquiv_PS2023_SideEffectOrder pins that the operand is evaluated
// exactly once in both forms.
func TestEquiv_PS2023_SideEffectOrder(t *testing.T) {
	count := 0
	g := func() string { count++; return "v" }
	if got := ps2023BeforeS(g()).Error(); got != "v" {
		t.Fatalf("before: got %q", got)
	}
	if count != 1 {
		t.Fatalf("before evaluated the operand %d times, want 1", count)
	}
	count = 0
	if got := ps2023After(g()).Error(); got != "v" {
		t.Fatalf("after: got %q", got)
	}
	if count != 1 {
		t.Fatalf("after evaluated the operand %d times, want 1", count)
	}
}
