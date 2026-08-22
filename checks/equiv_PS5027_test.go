package checks

// Runtime differential for PS5027: fmt.Sprintf(lit) vs the literal
// itself, for a string literal with no '%' byte. The fix's safety
// argument is fmt's own contract: doPrintf copies every byte of a
// verb-free format run verbatim into the pp buffer, and with exactly
// one argument (the format) and no verbs there is nothing else to
// print — so string(p.buf) IS the format string, byte for byte. There
// is no operand at all, hence no named-type/String()/Error()/
// Formatter, nil, float or evaluation-order concern. This suite pins:
//
//   - byte-identity of fmt.Sprintf(s) == s over adversarial verb-free
//     strings: empty, ASCII, multi-byte UTF-8, NUL bytes, invalid
//     UTF-8 (lone 0xFF, overlong 0xC0 0x80, truncated sequences),
//     utf8.RuneError, control characters, backslash escapes that fmt
//     must NOT interpret, and long strings (past any small-buffer
//     path);
//   - a randomized fuzz over %-free byte strings with a fixed seed;
//   - the divergence witnesses that motivate every guard: any '%'
//     (%%->%, a lone verb -> %!v(MISSING), a trailing lone '%'), and
//     extra arguments (-> %!(EXTRA ...)) — each shown to CHANGE the
//     output, which is why the no-'%' rule and the exactly-1-arg rule
//     are load-bearing;
//   - the perf premise: fmt.Sprintf allocates the fresh result string
//     on every call, the literal allocates nothing.

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// ps5027Sprintf is fmt.Sprintf through a function value — the exact
// runtime behavior of the Before-shape, deliberately called below with
// NON-constant formats and with extra arguments to pin fmt's divergent
// outputs. The indirection keeps go vet's printf checker from
// (correctly) rejecting what is intentionally adversarial test input.
var ps5027Sprintf func(format string, a ...any) string = fmt.Sprintf

func TestEquiv_PS5027_VerbFreeIdentity(t *testing.T) {
	cases := []string{
		"",
		"database connection failed",
		"disk is full",
		"tab\there and newline\nthere",
		"quote \" backslash \\ backtick `",
		"NUL \x00 embedded and trailing \x00",
		"\xff",             // invalid UTF-8: lone continuation-range byte
		"\xc0\x80",         // invalid UTF-8: overlong encoding of NUL
		"\xe4\xb8",         // invalid UTF-8: truncated multi-byte sequence
		"�",                // U+FFFD itself, encoded validly
		"世界 🚀 déjà vu",     // multi-byte UTF-8
		"\x01\x02\x1b[31m", // control bytes and an ANSI escape
		strings.Repeat("long-", 10_000),
		strings.Repeat("\x00\xff", 4096),
	}
	for _, s := range cases {
		if got := ps5027Sprintf(s); got != s {
			t.Errorf("fmt.Sprintf(%q) = %q, want the literal itself", s, got)
		}
	}
}

// TestEquiv_PS5027_RandomizedVerbFree fuzzes %-free byte strings with a
// fixed seed: any byte value except '%', any length up to 1 KiB.
func TestEquiv_PS5027_RandomizedVerbFree(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5027))
	for range 5000 {
		n := rng.Intn(1024)
		b := make([]byte, n)
		for i := range b {
			for {
				c := byte(rng.Intn(256))
				if c != '%' {
					b[i] = c
					break
				}
			}
		}
		s := string(b)
		if got := ps5027Sprintf(s); got != s {
			t.Fatalf("fmt.Sprintf(%q) = %q, want the literal itself", s, got)
		}
	}
}

// TestEquiv_PS5027_DivergenceWitnesses pins the concrete inputs the
// guards exclude — each one makes fmt.Sprintf return something OTHER
// than its format string, so each is why the match is narrow.
func TestEquiv_PS5027_DivergenceWitnesses(t *testing.T) {
	// Any '%' breaks identity: %% collapses, a verb with no operand
	// prints %!verb(MISSING), a trailing lone '%' prints %!(NOVERB).
	for _, w := range []struct{ format, want string }{
		{"100%% done", "100% done"},
		{"%d", "%!d(MISSING)"},
		{"%v", "%!v(MISSING)"},
		{"50%", "50%!(NOVERB)"},
	} {
		got := ps5027Sprintf(w.format)
		if got == w.format {
			t.Errorf("fmt.Sprintf(%q) returned its format verbatim — the no-'%%' guard would be unnecessary", w.format)
		}
		if got != w.want {
			t.Errorf("fmt.Sprintf(%q) = %q, want %q (divergence witness drifted)", w.format, got, w.want)
		}
	}
	// Extra arguments with a verb-free format append %!(EXTRA ...).
	if got, want := ps5027Sprintf("static", 42), "static%!(EXTRA int=42)"; got != want {
		t.Errorf("fmt.Sprintf(\"static\", 42) = %q, want %q (the exactly-1-arg guard rests on this)", got, want)
	}
}

// TestEquiv_PS5027_Allocs pins the perf premise: the literal is a
// compile-time constant and allocates nothing, while fmt.Sprintf
// heap-allocates the fresh result string on every call.
func TestEquiv_PS5027_Allocs(t *testing.T) {
	const lit = "database connection failed"
	var sink string
	if avg := testing.AllocsPerRun(100, func() { sink = lit }); avg != 0 {
		t.Errorf("the literal allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(100, func() { sink = ps5027Sprintf(lit) }); avg < 1 {
		t.Errorf("fmt.Sprintf(lit) allocates %v times per run, want >= 1 (the fresh result string)", avg)
	}
	_ = sink
}
