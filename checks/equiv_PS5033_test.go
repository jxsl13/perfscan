package checks

// Runtime differential for PS5033: fmt.Append(buf, s) vs the
// append(buf, s...) form the fix emits — for every operand whose static
// type is EXACTLY the predeclared string. The fix's safety argument:
// fmt.Append appends fmt.Sprint(a...); with ONE operand Sprint's
// between-operand spacing rule never applies, and %v over a value whose
// dynamic type is exactly string takes fmt's reflection-free fast path
// (printArg's `case string` -> fmtString -> fmtS with no flags set) that
// writes the string's raw bytes verbatim — no quoting, no UTF-8
// validation, no padding. That is byte-for-byte what the builtin
// append's append-string-to-[]byte special case copies, and both forms
// grow buf with the same builtin append, so length, contents AND
// capacity/aliasing behavior coincide. This suite pins that claim over:
//
//   - adversarial operand strings: empty, ASCII, NUL bytes, every kind
//     of invalid UTF-8 (bare continuation bytes, truncated and overlong
//     sequences, surrogate encodings, 0xFF), fmt-metacharacters (%s, %v,
//     %%, %!x(MISSING)) that must pass through UNinterpreted (Append is
//     not Appendf), a real U+FFFD, multi-byte runes, and long strings;
//   - adversarial destinations: nil, empty-non-nil, preloaded with
//     arbitrary bytes (invalid UTF-8 included), with and without spare
//     capacity — crossed with every operand;
//   - aliasing/growth pins: when buf has spare capacity BOTH forms write
//     into the same backing array (fmt.Append's final `b = append(b,
//     p.buf...)` is the same builtin append the fix emits), and a
//     preloaded prefix is never disturbed;
//   - the guards' load-bearing exclusions, pinned as divergence
//     WITNESSES: a named string type with a String() method (fmt honors
//     it, append would not), a []byte operand (%v prints the decimal
//     slice representation), nil ("<nil>"), and two operands of
//     non-string-adjacent kinds (the spacing rule) — each asserted to
//     actually diverge, so the guard is load-bearing, not folklore;
//   - the perf premise: the append form never allocates when buf has
//     capacity, while fmt.Append's interface box does.

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// ps5033Before is the exact Before-shape of the check; ps5033After is the
// exact After-shape the fix emits.
func ps5033Before(buf []byte, s string) []byte { return fmt.Append(buf, s) }
func ps5033After(buf []byte, s string) []byte  { return append(buf, s...) }

// ps5033Operands are the operand strings the differential crosses with
// every destination: empty, ASCII, NULs, every flavor of invalid UTF-8,
// fmt metacharacters (which Append must NOT interpret), well-formed
// multi-byte runes, a real U+FFFD, and long payloads.
var ps5033Operands = []string{
	"",
	"a",
	"hello, world",
	"\x00",
	"a\x00b\x00",
	"\x80",            // bare continuation byte
	"\xff\xfe",        // never valid UTF-8
	"\xe2\x80",        // truncated 3-byte sequence
	"\xc0\xaf",        // overlong encoding
	"\xed\xa0\x80",    // surrogate encoding
	"\xf0\x9f\x98",    // truncated 4-byte sequence
	"%s",              // must pass through raw: Append is not Appendf
	"%v %d %%",        // more metacharacters, raw
	"%!s(MISSING)",    // fmt's own error syntax, raw
	"é—😀\u00a0\u200b", // multi-byte runes incl NBSP and ZWSP escapes
	"�",               // a real U+FFFD
	"ab�cd\xffef",     // U+FFFD next to invalid bytes
	strings.Repeat("x", 1000),
	strings.Repeat("\xe2\x80\x94", 500),      // long, multi-byte
	strings.Repeat("payload=\x00\xff ", 256), // long, NULs and invalid bytes
}

// ps5033Dests builds the destination []byte shapes crossed with every
// operand: nil, empty-non-nil, preloaded (ASCII and invalid-UTF-8
// prefixes), each without spare capacity and with plenty.
func ps5033Dests() [][]byte {
	prefixes := []string{"", "seed", "\xff\x00\x80pre"}
	var out [][]byte
	for _, p := range prefixes {
		tight := []byte(p)                         // len == cap (or nil for "")
		roomy := make([]byte, len(p), len(p)+4096) // plenty of spare capacity
		copy(roomy, p)
		out = append(out, tight, roomy)
	}
	out = append(out, nil, []byte{}) // explicit nil and empty-non-nil
	return out
}

func TestEquiv_PS5033_OperandDestCross(t *testing.T) {
	for _, s := range ps5033Operands {
		for _, buf := range ps5033Dests() {
			// Fresh copies: both forms may write into spare capacity.
			b1 := append([]byte(nil), buf...)
			b2 := append([]byte(nil), buf...)
			got1 := ps5033Before(b1, s)
			got2 := ps5033After(b2, s)
			if string(got1) != string(got2) {
				t.Fatalf("divergence on (buf=%q, s=%q): fmt.Append=%q append=%q", buf, s, got1, got2)
			}
			if len(got1) != len(buf)+len(s) {
				t.Fatalf("length drift on (buf=%q, s=%q): got %d want %d", buf, s, len(got1), len(buf)+len(s))
			}
		}
	}
}

// TestEquiv_PS5033_AliasingAndGrowth pins that BOTH forms are the same
// builtin append over buf: with spare capacity each writes into buf's
// existing backing array (no reallocation), and without capacity each
// leaves the original backing array untouched.
func TestEquiv_PS5033_AliasingAndGrowth(t *testing.T) {
	s := "payload\xff\x00"
	for _, form := range []struct {
		name string
		fn   func([]byte, string) []byte
	}{
		{"fmt.Append", ps5033Before},
		{"append", ps5033After},
	} {
		// Spare capacity: the result aliases buf's backing array.
		backing := make([]byte, 4, 64)
		copy(backing, "seed")
		got := form.fn(backing, s)
		if &got[0] != &backing[0] {
			t.Fatalf("%s with spare capacity reallocated; both forms must be the builtin append over buf", form.name)
		}
		if string(backing) != "seed" {
			t.Fatalf("%s disturbed the preloaded prefix: %q", form.name, backing)
		}
		// No spare capacity: the original backing array stays untouched.
		tight := []byte("tight")
		tight = tight[:len(tight):len(tight)]
		snapshot := string(tight)
		got = form.fn(tight, s)
		if string(tight) != snapshot {
			t.Fatalf("%s modified the full source slice in place: %q", form.name, tight)
		}
		if string(got) != snapshot+s {
			t.Fatalf("%s grew wrong: %q", form.name, got)
		}
	}
}

func TestEquiv_PS5033_RandomizedLong(t *testing.T) {
	rng := rand.New(rand.NewSource(0x25033))
	for range 20000 {
		s := make([]byte, rng.Intn(300))
		for i := range s {
			s[i] = byte(rng.Intn(256))
		}
		n := rng.Intn(64)
		buf := make([]byte, n, n+rng.Intn(512))
		for i := range buf {
			buf[i] = byte(rng.Intn(256))
		}
		b1 := append([]byte(nil), buf...)
		b2 := append([]byte(nil), buf...)
		got1 := ps5033Before(b1, string(s))
		got2 := ps5033After(b2, string(s))
		if string(got1) != string(got2) {
			t.Fatalf("randomized divergence on (buf=%q, s=%q): fmt.Append=%q append=%q", buf, s, got1, got2)
		}
	}
}

// ps5033NamedStr is the divergence witness for the fix's
// predeclared-string guard: %v honors its String() method, append copies
// its raw bytes.
type ps5033NamedStr string

func (ps5033NamedStr) String() string { return "STRINGER" }

// TestEquiv_PS5033_GuardWitnesses pins that every advisory/negative guard
// excludes a shape that ACTUALLY diverges — the guards are load-bearing.
func TestEquiv_PS5033_GuardWitnesses(t *testing.T) {
	// A NAMED string with String(): fmt honors the method, append the bytes.
	var n ps5033NamedStr = "raw"
	if got := string(fmt.Append(nil, n)); got != "STRINGER" {
		t.Fatalf("fmt.Append over a Stringer-carrying named string = %q, want %q — re-audit the named-type guard", got, "STRINGER")
	}
	if got := string(append([]byte(nil), n...)); got != "raw" {
		t.Fatalf("append over a named string = %q, want the raw bytes %q", got, "raw")
	}
	// A []byte operand: %v is the decimal slice representation.
	if got := string(fmt.Append(nil, []byte("hi"))); got != "[104 105]" {
		t.Fatalf("fmt.Append over a []byte = %q, want the decimal slice form %q — re-audit the []byte exclusion", got, "[104 105]")
	}
	// nil prints "<nil>".
	if got := string(fmt.Append(nil, nil)); got != "<nil>" {
		t.Fatalf("fmt.Append over nil = %q, want %q", got, "<nil>")
	}
	// Two non-string operands engage Sprint's spacing rule: no
	// concatenation-of-appends can reproduce the injected space blindly.
	if got := string(fmt.Append(nil, 1, 2)); got != "1 2" {
		t.Fatalf("fmt.Append(nil, 1, 2) = %q, want %q — the multi-operand exclusion's premise moved", got, "1 2")
	}
	// Two STRING operands get no space — but the single-operand guard
	// keeps PS5033 out of multi-operand calls entirely; pin the premise
	// that spacing DEPENDS on operand kinds, so the exclusion stays.
	if got := string(fmt.Append(nil, "a", "b")); got != "ab" {
		t.Fatalf("fmt.Append(nil, \"a\", \"b\") = %q, want %q", got, "ab")
	}
}

// TestEquiv_PS5033_AfterSideZeroAlloc pins the perf premise: with spare
// capacity the append form never allocates, while fmt.Append pays the
// interface box for the string operand on every call.
func TestEquiv_PS5033_AfterSideZeroAlloc(t *testing.T) {
	buf := make([]byte, 0, 4096)
	s := strings.Repeat("x", 100)
	var sink []byte
	allocs := testing.AllocsPerRun(100, func() {
		sink = append(buf[:0], s...)
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("append(buf, s...) allocated %v times per run into spare capacity; PS5033's zero-alloc premise no longer holds", allocs)
	}
}
