package checks

// Runtime differential for PS2040: fmt.Append(buf, a, b, ...) over plain
// strings versus the nested chain append(append(buf, a...), b...). The fix's
// safety argument is Sprint's spacing rule: a space is inserted between two
// operands only when NEITHER is a string, so with EVERY operand a string no
// separator is ever inserted and fmt.Append appends exactly a+b+...+z — the
// bytes the builtin append chain writes. %v over a value whose dynamic type is
// exactly string takes fmt's reflection-free fast path that copies the raw
// bytes verbatim (invalid UTF-8 and empties included). This suite pins:
//
//   - byte identity, result length and nil-ness parity for 2-, 3- and
//     4-operand shapes over adversarial operands (empty, NUL, invalid UTF-8,
//     a lone surrogate, 4 KiB, random byte strings with a fixed seed) crossed
//     with adversarial destinations (nil, non-nil empty, seeded, spare
//     capacity, zero capacity);
//   - evaluation order and count parity for the shapes the fix accepts (an
//     arbitrary buf expression and an arbitrary FIRST operand, both calls);
//   - the divergences the fix's gates exclude, reproduced positively: a NAMED
//     string operand whose Stringer %v honors and append does not (excluded
//     by the predeclared-string gate), a later operand call observing buf's
//     array between the chain's writes (excluded by the inert gate), and —
//     why non-string operands are never even reported — Sprint's space
//     between two adjacent non-strings and %v's decimal []byte form;
//   - the perf premise: the After shape is allocation-free into existing
//     capacity while fmt.Append boxes each operand.

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func ps2040Before2(buf []byte, a, b string) []byte { return fmt.Append(buf, a, b) }
func ps2040After2(buf []byte, a, b string) []byte  { return append(append(buf, a...), b...) }
func ps2040Before3(buf []byte, a, b, c string) []byte {
	return fmt.Append(buf, a, b, c)
}
func ps2040After3(buf []byte, a, b, c string) []byte {
	return append(append(append(buf, a...), b...), c...)
}
func ps2040Before4(buf []byte, a, b, c, d string) []byte {
	return fmt.Append(buf, a, b, c, d)
}
func ps2040After4(buf []byte, a, b, c, d string) []byte {
	return append(append(append(append(buf, a...), b...), c...), d...)
}

func ps2040Bufs() [][]byte {
	spare := make([]byte, 2, 64)
	copy(spare, "sp")
	return [][]byte{
		nil,
		{},
		[]byte("seed"),
		spare,
		make([]byte, 0, 1), // forces repeated growth in the chain
	}
}

func TestEquiv_PS2040_ByteIdentity(t *testing.T) {
	ops := []string{
		"", "a", " ", "hello",
		"\x00", "a\x00b",
		"\xff\xfe\x80",       // invalid UTF-8 — %v and append both copy the raw bytes
		string(rune(0xD800)), // lone surrogate literal encodes as RuneError
		"héllo, 世界", "\U0001F600",
		strings.Repeat("q", 4096),
	}
	rng := rand.New(rand.NewSource(0x2040))
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
	// The full adversarial cross for two operands, plus randomized triples and
	// quadruples.
	for _, a := range ops[:11] {
		for _, b := range ops[:11] {
			for _, buf0 := range ps2040Bufs() {
				before := ps2040Before2(append([]byte(nil), buf0...), a, b)
				after := ps2040After2(append([]byte(nil), buf0...), a, b)
				check(buf0, before, after, fmt.Sprintf("2-op (%q,%q)", a, b))
			}
		}
	}
	for range 3000 {
		a, b, c, d := pick(), pick(), pick(), pick()
		for _, buf0 := range ps2040Bufs() {
			before := ps2040Before2(append([]byte(nil), buf0...), a, b)
			after := ps2040After2(append([]byte(nil), buf0...), a, b)
			check(buf0, before, after, "2-op random")
			before = ps2040Before3(append([]byte(nil), buf0...), a, b, c)
			after = ps2040After3(append([]byte(nil), buf0...), a, b, c)
			check(buf0, before, after, "3-op random")
			before = ps2040Before4(append([]byte(nil), buf0...), a, b, c, d)
			after = ps2040After4(append([]byte(nil), buf0...), a, b, c, d)
			check(buf0, before, after, "4-op random")
		}
	}
}

// TestEquiv_PS2040_EvaluationOrder pins that for the shapes the fix accepts —
// an arbitrary buf expression and an arbitrary FIRST operand (both may be
// calls), later operands inert — both forms run the same user code in the same
// order, exactly once. The chain's first write to buf happens after both calls
// in both forms, so nothing can observe a difference.
func TestEquiv_PS2040_EvaluationOrder(t *testing.T) {
	var trace []string
	bufFn := func() []byte { trace = append(trace, "buf"); return make([]byte, 0, 8) }
	aFn := func() string { trace = append(trace, "a"); return "aa" }
	b := "bb"

	trace = nil
	before := fmt.Append(bufFn(), aFn(), b)
	beforeTrace := strings.Join(trace, ",")

	trace = nil
	after := append(append(bufFn(), aFn()...), b...)
	afterTrace := strings.Join(trace, ",")

	if beforeTrace != afterTrace {
		t.Fatalf("evaluation order diverges: before=%s after=%s", beforeTrace, afterTrace)
	}
	if beforeTrace != "buf,a" {
		t.Fatalf("unexpected evaluation trace %q, want buf,a", beforeTrace)
	}
	if string(before) != string(after) || string(after) != "aabb" {
		t.Fatalf("results diverge: before=%q after=%q", before, after)
	}
}

// TestEquiv_PS2040_NamedStringerDivergence reproduces the divergence the
// predeclared-string gate excludes: a NAMED string type implementing
// fmt.Stringer. %v honors String() (and would honor Format() likewise);
// the builtin append copies the underlying bytes.
type ps2040Named string

func (ps2040Named) String() string { return "STRINGER" }

func TestEquiv_PS2040_NamedStringerDivergence(t *testing.T) {
	s := ps2040Named("raw")
	before := string(fmt.Append(nil, "a", s))
	if before != "aSTRINGER" {
		t.Fatalf("fmt.Append over a named Stringer = %q, want %q", before, "aSTRINGER")
	}
	after := string(append(append([]byte(nil), "a"...), s...))
	if after != "araw" {
		t.Fatalf("append chain over a named Stringer = %q, want %q (append copies the raw bytes)", after, "araw")
	}
	if before == after {
		t.Fatal("expected the named-Stringer shapes to diverge — the predeclared-string gate would be unnecessary")
	}
}

// TestEquiv_PS2040_LaterCallDivergence reproduces the divergence the inert
// gate excludes: a LATER operand that runs user code observing buf's array.
// The chain writes into buf's capacity between evaluating the first and
// second operands; fmt.Append writes only after evaluating everything.
func TestEquiv_PS2040_LaterCallDivergence(t *testing.T) {
	arrA := make([]byte, 8)
	copy(arrA, "XXXXXXXX")
	fA := func() string { return string(arrA[2:4]) }
	before := string(fmt.Append(arrA[:2], "ab", fA()))
	if before != "XXabXX" {
		t.Fatalf("fmt.Append with an observing later call = %q, want %q", before, "XXabXX")
	}

	arrB := make([]byte, 8)
	copy(arrB, "XXXXXXXX")
	fB := func() string { return string(arrB[2:4]) }
	after := string(append(append(arrB[:2], "ab"...), fB()...))
	if after != "XXabab" {
		t.Fatalf("append chain with an observing later call = %q, want %q (the call sees the first write)", after, "XXabab")
	}
	if before == after {
		t.Fatal("expected the observing-call shapes to diverge — the inert gate would be unnecessary")
	}
}

// TestEquiv_PS2040_NonStringNeverReported pins the two behaviors that keep
// non-string operands out of the check's scope entirely: Sprint inserts a
// space between two ADJACENT NON-STRING operands, and %v over a []byte prints
// the decimal slice representation.
func TestEquiv_PS2040_NonStringNeverReported(t *testing.T) {
	if got := string(fmt.Append(nil, 1, 2, "x")); got != "1 2x" {
		t.Fatalf("fmt.Append(nil, 1, 2, \"x\") = %q, want %q (space between adjacent non-strings)", got, "1 2x")
	}
	if got := string(fmt.Append(nil, "a", []byte("hi"))); got != "a[104 105]" {
		t.Fatalf("fmt.Append(nil, \"a\", []byte(\"hi\")) = %q, want %q (decimal slice form)", got, "a[104 105]")
	}
}

// TestEquiv_PS2040_AfterAllocProfile pins the perf premise: into existing
// capacity the chain allocates nothing, while fmt.Append boxes each string
// operand into an interface. The operands live at package scope (as in the
// benchmark) so the compiler cannot prove the boxes don't escape.
var (
	ps2040AllocBuf = make([]byte, 0, 256)
	ps2040AllocA   = "hello, "
	ps2040AllocB   = "world"
	ps2040AllocOut []byte
)

func TestEquiv_PS2040_AfterAllocProfile(t *testing.T) {
	if avg := testing.AllocsPerRun(200, func() {
		ps2040AllocOut = ps2040After2(ps2040AllocBuf[:0], ps2040AllocA, ps2040AllocB)
	}); avg != 0 {
		t.Errorf("append chain into existing capacity allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(200, func() {
		ps2040AllocOut = ps2040Before2(ps2040AllocBuf[:0], ps2040AllocA, ps2040AllocB)
	}); avg < 1 {
		t.Logf("fmt.Append allocates %v times per run — the compiler learned to elide the boxes; the win claim may need re-framing", avg)
	}
}
