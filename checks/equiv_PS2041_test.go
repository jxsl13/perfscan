package checks

// Runtime differential for PS2041: fmt.Appendln(buf, a, b) over plain strings
// versus the nested chain append(append(append(append(buf, a...), ' '), b...), '\n').
// The fix's safety argument: doPrintln is UNCONDITIONAL and format-free — it
// writes exactly one ' ' between adjacent operands and exactly one trailing
// '\n', with no format-string interpretation — and %v over a value of EXACTLY
// the predeclared string type takes fmt's reflection-free fast path that
// writes the string's raw bytes verbatim (invalid UTF-8 and empties
// included); fmt.Appendln then appends the formatted buffer to buf — exactly
// the bytes the builtin append chain writes. This suite pins:
//
//   - byte identity, result length and nil-ness parity for 1-, 2- and
//     3-operand shapes over adversarial operands (empty, NUL, every flavor of
//     invalid UTF-8, fmt metacharacters that must pass through UNinterpreted,
//     a lone surrogate, 4 KiB, random byte strings with a fixed seed) crossed
//     with adversarial destinations (nil, non-nil empty, seeded, spare
//     capacity, zero capacity, partial capacity that forces the chain to grow
//     mid-way);
//   - the UNCONDITIONAL separator rule itself (empty operands still get their
//     ' ' and '\n' — unlike Sprint/Append's conditional spacing);
//   - evaluation order and count parity for the shapes the fix accepts (an
//     arbitrary buf expression and an arbitrary FIRST operand, both calls);
//   - the divergences the fix's gates exclude, reproduced positively: a NAMED
//     string operand whose String() %v honors (excluded by the
//     predeclared-string gate), a []byte operand (%v prints the decimal slice
//     form — excluded from reporting entirely), nil ("<nil>"), and a later
//     operand call observing buf's array between the chain's writes (excluded
//     by the inert gate);
//   - the perf premise: the After shape is allocation-free into existing
//     capacity while fmt.Appendln boxes each operand.

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func ps2041Before1(buf []byte, a string) []byte { return fmt.Appendln(buf, a) }
func ps2041After1(buf []byte, a string) []byte  { return append(append(buf, a...), '\n') }
func ps2041Before2(buf []byte, a, b string) []byte {
	return fmt.Appendln(buf, a, b)
}
func ps2041After2(buf []byte, a, b string) []byte {
	return append(append(append(append(buf, a...), ' '), b...), '\n')
}
func ps2041Before3(buf []byte, a, b, c string) []byte {
	return fmt.Appendln(buf, a, b, c)
}
func ps2041After3(buf []byte, a, b, c string) []byte {
	return append(append(append(append(append(append(buf, a...), ' '), b...), ' '), c...), '\n')
}

func ps2041Bufs() [][]byte {
	spare := make([]byte, 2, 64)
	copy(spare, "sp")
	partial := make([]byte, 2, 6) // fits the first operand but not the rest:
	copy(partial, "pa")           // forces the chain to grow mid-way
	return [][]byte{
		nil,
		{},
		[]byte("seed"),
		spare,
		partial,
		make([]byte, 0, 1), // forces repeated growth in the chain
	}
}

func TestEquiv_PS2041_ByteIdentity(t *testing.T) {
	ops := []string{
		"", "a", " ", "hello",
		"\x00", "a\x00b",
		"\xff\xfe\x80",       // invalid UTF-8 — %v and append both copy the raw bytes
		string(rune(0xD800)), // lone surrogate literal encodes as RuneError
		"%s %v %%",           // fmt metacharacters must pass through raw: Appendln is format-free
		"héllo, 世界", "\U0001F600",
		strings.Repeat("q", 4096),
	}
	rng := rand.New(rand.NewSource(0x2041))
	for range 500 {
		b := make([]byte, rng.Intn(48))
		rng.Read(b)
		ops = append(ops, string(b))
	}
	pick := func() string { return ops[rng.Intn(len(ops))] }
	check := func(buf0 []byte, before, after []byte, label string) {
		t.Helper()
		if string(before) != string(after) {
			t.Fatalf("%s diverges over buf=%q: before=%q after=%q", label, buf0, before, after)
		}
		if len(before) != len(after) {
			t.Fatalf("%s length diverges: %d vs %d", label, len(before), len(after))
		}
		if (before == nil) != (after == nil) {
			t.Fatalf("%s nil-ness diverges: before==nil %v, after==nil %v", label, before == nil, after == nil)
		}
	}
	// The full adversarial cross for one and two operands, plus randomized triples.
	for _, a := range ops[:12] {
		for _, buf0 := range ps2041Bufs() {
			before := ps2041Before1(append([]byte(nil), buf0...), a)
			after := ps2041After1(append([]byte(nil), buf0...), a)
			check(buf0, before, after, fmt.Sprintf("1-op (%q)", a))
		}
		for _, b := range ops[:12] {
			for _, buf0 := range ps2041Bufs() {
				before := ps2041Before2(append([]byte(nil), buf0...), a, b)
				after := ps2041After2(append([]byte(nil), buf0...), a, b)
				check(buf0, before, after, fmt.Sprintf("2-op (%q,%q)", a, b))
			}
		}
	}
	for range 3000 {
		a, b, c := pick(), pick(), pick()
		for _, buf0 := range ps2041Bufs() {
			before := ps2041Before2(append([]byte(nil), buf0...), a, b)
			after := ps2041After2(append([]byte(nil), buf0...), a, b)
			check(buf0, before, after, "2-op random")
			before = ps2041Before3(append([]byte(nil), buf0...), a, b, c)
			after = ps2041After3(append([]byte(nil), buf0...), a, b, c)
			check(buf0, before, after, "3-op random")
		}
	}
}

// TestEquiv_PS2041_UnconditionalSeparators pins the premise that doPrintln's
// separator rule — unlike Sprint/Append's — does NOT depend on the operands:
// empty strings still get exactly one ' ' between and one '\n' after. If this
// ever changed, the whole rewrite would be wrong.
func TestEquiv_PS2041_UnconditionalSeparators(t *testing.T) {
	for _, tc := range []struct {
		got, want string
	}{
		{string(fmt.Appendln(nil, "a", "b")), "a b\n"},
		{string(fmt.Appendln(nil, "", "")), " \n"},
		{string(fmt.Appendln(nil, "")), "\n"},
		{string(fmt.Appendln(nil, "a", "", "b")), "a  b\n"},
	} {
		if tc.got != tc.want {
			t.Fatalf("fmt.Appendln separator rule moved: got %q, want %q — PS2041's premise is broken", tc.got, tc.want)
		}
	}
	// And fmt.Append's rule really is CONDITIONAL (why Appendln, not Append,
	// admits this blind rewrite): two strings get no space.
	if got := string(fmt.Append(nil, "a", "b")); got != "ab" {
		t.Fatalf("fmt.Append(nil, \"a\", \"b\") = %q, want %q", got, "ab")
	}
}

// TestEquiv_PS2041_EvaluationOrder pins that for the shapes the fix accepts —
// an arbitrary buf expression and an arbitrary FIRST operand (both may be
// calls), later operands inert — both forms run the same user code in the
// same order, exactly once. The chain's first write to buf happens after both
// calls in both forms, so nothing can observe a difference.
func TestEquiv_PS2041_EvaluationOrder(t *testing.T) {
	var trace []string
	bufFn := func() []byte { trace = append(trace, "buf"); return make([]byte, 0, 8) }
	aFn := func() string { trace = append(trace, "a"); return "aa" }
	b := "bb"

	trace = nil
	before := fmt.Appendln(bufFn(), aFn(), b)
	beforeTrace := strings.Join(trace, ",")

	trace = nil
	after := append(append(append(append(bufFn(), aFn()...), ' '), b...), '\n')
	afterTrace := strings.Join(trace, ",")

	if beforeTrace != afterTrace {
		t.Fatalf("evaluation order diverges: before=%s after=%s", beforeTrace, afterTrace)
	}
	if beforeTrace != "buf,a" {
		t.Fatalf("unexpected evaluation trace %q, want buf,a", beforeTrace)
	}
	if string(before) != string(after) || string(after) != "aa bb\n" {
		t.Fatalf("results diverge: before=%q after=%q", before, after)
	}
}

// ps2041NamedStr is the divergence witness for the fix's predeclared-string
// gate: %v honors its String() method, append copies its raw bytes.
type ps2041NamedStr string

func (ps2041NamedStr) String() string { return "STRINGER" }

// TestEquiv_PS2041_GuardWitnesses pins that every advisory/negative guard
// excludes a shape that ACTUALLY diverges — the guards are load-bearing.
func TestEquiv_PS2041_GuardWitnesses(t *testing.T) {
	// A NAMED string with String(): fmt honors the method, append the bytes.
	var n ps2041NamedStr = "raw"
	if got := string(fmt.Appendln(nil, n)); got != "STRINGER\n" {
		t.Fatalf("fmt.Appendln over a Stringer-carrying named string = %q, want %q — re-audit the named-type guard", got, "STRINGER\n")
	}
	if got := string(append(append([]byte(nil), n...), '\n')); got != "raw\n" {
		t.Fatalf("append over a named string = %q, want the raw bytes %q", got, "raw\n")
	}
	// A []byte operand: %v is the decimal slice representation — excluded
	// from reporting entirely.
	if got := string(fmt.Appendln(nil, []byte("hi"))); got != "[104 105]\n" {
		t.Fatalf("fmt.Appendln over a []byte = %q, want the decimal slice form %q — re-audit the []byte exclusion", got, "[104 105]\n")
	}
	// nil prints "<nil>".
	if got := string(fmt.Appendln(nil, nil)); got != "<nil>\n" {
		t.Fatalf("fmt.Appendln over nil = %q, want %q", got, "<nil>\n")
	}
	// Zero operands append just "\n" — pinned so the out-of-scope negative
	// stays honest.
	if got := string(fmt.Appendln([]byte("x"))); got != "x\n" {
		t.Fatalf("fmt.Appendln with zero operands = %q, want %q", got, "x\n")
	}
}

// TestEquiv_PS2041_LaterCallDivergence reproduces the divergence the inert
// gate excludes: a LATER operand that runs user code observing buf's array.
// The chain writes into buf's capacity between evaluating the first and
// second operands; fmt.Appendln evaluates every argument at the call site,
// before its only write.
func TestEquiv_PS2041_LaterCallDivergence(t *testing.T) {
	arrA := make([]byte, 8)
	copy(arrA, "XXXXXXXX")
	fA := func() string { return string(arrA[2:4]) }
	before := string(fmt.Appendln(arrA[:2], "ab", fA()))
	if before != "XXab XX\n" {
		t.Fatalf("fmt.Appendln with an observing later call = %q, want %q", before, "XXab XX\n")
	}

	arrB := make([]byte, 8)
	copy(arrB, "XXXXXXXX")
	fB := func() string { return string(arrB[2:4]) }
	after := string(append(append(append(append(arrB[:2], "ab"...), ' '), fB()...), '\n'))
	if after != "XXab ab\n" {
		t.Fatalf("append chain with an observing later call = %q, want %q (the call sees the first writes)", after, "XXab ab\n")
	}
	if before == after {
		t.Fatal("expected the observing-call shapes to diverge — the inert gate would be unnecessary")
	}
}

// TestEquiv_PS2041_AfterAllocProfile pins the perf premise: into existing
// capacity the chain allocates nothing, while fmt.Appendln boxes each string
// operand into an interface. The operands live at package scope (as in the
// benchmark) so the compiler cannot prove the boxes don't escape.
var (
	ps2041AllocBuf = make([]byte, 0, 256)
	ps2041AllocA   = "hello,"
	ps2041AllocB   = "world"
	ps2041AllocOut []byte
)

func TestEquiv_PS2041_AfterAllocProfile(t *testing.T) {
	if avg := testing.AllocsPerRun(200, func() {
		ps2041AllocOut = ps2041After2(ps2041AllocBuf[:0], ps2041AllocA, ps2041AllocB)
	}); avg != 0 {
		t.Errorf("append chain into existing capacity allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(200, func() {
		ps2041AllocOut = ps2041Before2(ps2041AllocBuf[:0], ps2041AllocA, ps2041AllocB)
	}); avg < 1 {
		t.Logf("fmt.Appendln allocates %v times per run — the compiler learned to elide the boxes; the win claim may need re-framing", avg)
	}
}
