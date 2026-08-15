package checks

// Runtime differential for PS2033: fmt.Appendf(buf, "%s%s", a, b) over plain
// strings versus the nested chain append(append(buf, a...), b...). The fix's
// safety argument is that %s with no flag/width over a value of EXACTLY the
// predeclared string type writes that string's bytes verbatim (invalid UTF-8
// and empties included), and fmt.Appendf then appends the formatted buffer to
// buf — exactly the bytes the builtin append chain writes. This suite pins:
//
//   - byte identity, result length and nil-ness parity for 2- and 3-operand
//     shapes over adversarial operands (empty, NUL, invalid UTF-8, a lone
//     surrogate, 4 KiB, random byte strings with a fixed seed) crossed with
//     adversarial destinations (nil, non-nil empty, seeded, spare capacity,
//     zero capacity);
//   - evaluation order and count parity for the shapes the fix accepts (an
//     arbitrary buf expression and an arbitrary FIRST operand, both calls);
//   - the two divergences the fix's gates exclude, reproduced positively:
//     a []byte operand aliasing buf's spare capacity (excluded by the
//     strings-only operand gate) and a later operand call observing buf's
//     array between the chain's writes (excluded by the inert gate);
//   - the perf premise: the After shape is allocation-free into existing
//     capacity while fmt.Appendf boxes each operand.

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func ps2033Before2(buf []byte, a, b string) []byte { return fmt.Appendf(buf, "%s%s", a, b) }
func ps2033After2(buf []byte, a, b string) []byte  { return append(append(buf, a...), b...) }
func ps2033Before3(buf []byte, a, b, c string) []byte {
	return fmt.Appendf(buf, "%s%s%s", a, b, c)
}
func ps2033After3(buf []byte, a, b, c string) []byte {
	return append(append(append(buf, a...), b...), c...)
}

func ps2033Bufs() [][]byte {
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

func TestEquiv_PS2033_ByteIdentity(t *testing.T) {
	ops := []string{
		"", "a", " ", "hello",
		"\x00", "a\x00b",
		"\xff\xfe\x80",       // invalid UTF-8 — %s and append both copy the raw bytes
		string(rune(0xD800)), // lone surrogate literal encodes as RuneError
		"héllo, 世界", "\U0001F600",
		strings.Repeat("q", 4096),
	}
	rng := rand.New(rand.NewSource(0x2033))
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
	// The full adversarial cross for two operands, plus randomized triples.
	for _, a := range ops[:11] {
		for _, b := range ops[:11] {
			for _, buf0 := range ps2033Bufs() {
				before := ps2033Before2(append([]byte(nil), buf0...), a, b)
				after := ps2033After2(append([]byte(nil), buf0...), a, b)
				check(buf0, before, after, fmt.Sprintf("2-op (%q,%q)", a, b))
			}
		}
	}
	for range 3000 {
		a, b, c := pick(), pick(), pick()
		for _, buf0 := range ps2033Bufs() {
			before := ps2033Before2(append([]byte(nil), buf0...), a, b)
			after := ps2033After2(append([]byte(nil), buf0...), a, b)
			check(buf0, before, after, "2-op random")
			before = ps2033Before3(append([]byte(nil), buf0...), a, b, c)
			after = ps2033After3(append([]byte(nil), buf0...), a, b, c)
			check(buf0, before, after, "3-op random")
		}
	}
}

// TestEquiv_PS2033_EvaluationOrder pins that for the shapes the fix accepts —
// an arbitrary buf expression and an arbitrary FIRST operand (both may be
// calls), later operands inert — both forms run the same user code in the same
// order, exactly once. The chain's first write to buf happens after both calls
// in both forms, so nothing can observe a difference.
func TestEquiv_PS2033_EvaluationOrder(t *testing.T) {
	var trace []string
	bufFn := func() []byte { trace = append(trace, "buf"); return make([]byte, 0, 8) }
	aFn := func() string { trace = append(trace, "a"); return "aa" }
	b := "bb"

	trace = nil
	before := fmt.Appendf(bufFn(), "%s%s", aFn(), b)
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

// TestEquiv_PS2033_ByteSliceOperandDivergence reproduces the divergence the
// strings-only operand gate excludes: a []byte operand whose bytes alias buf's
// spare capacity. fmt.Appendf reads every operand before writing to buf; the
// chain's FIRST append overwrites the aliased bytes before the second append
// reads them. Single-operand PS2141 can fix []byte operands (one read, one
// write, memmove-safe); the multi-operand chain cannot.
func TestEquiv_PS2033_ByteSliceOperandDivergence(t *testing.T) {
	shape := func() ([]byte, []byte) {
		mem := make([]byte, 4, 16)
		copy(mem, "abcd")
		return mem[:2], mem[2:4] // buf and an operand aliasing its spare capacity
	}
	buf, bs := shape()
	before := fmt.Appendf(buf, "%s%s", "XY", bs)
	if string(before) != "abXYcd" {
		t.Fatalf("fmt.Appendf over aliasing []byte = %q, want %q", before, "abXYcd")
	}
	buf, bs = shape()
	after := append(append(buf, "XY"...), bs...)
	if string(after) != "abXYXY" {
		t.Fatalf("append chain over aliasing []byte = %q, want %q (the first append clobbers the operand)", after, "abXYXY")
	}
	if string(before) == string(after) {
		t.Fatal("expected the aliasing []byte shapes to diverge — the operand gate would be unnecessary")
	}
}

// TestEquiv_PS2033_LaterCallDivergence reproduces the divergence the inert
// gate excludes: a LATER operand that runs user code observing buf's array.
// The chain writes into buf's capacity between evaluating the first and
// second operands; fmt.Appendf writes only after evaluating everything.
func TestEquiv_PS2033_LaterCallDivergence(t *testing.T) {
	arrA := make([]byte, 8)
	copy(arrA, "XXXXXXXX")
	fA := func() string { return string(arrA[2:4]) }
	before := string(fmt.Appendf(arrA[:2], "%s%s", "ab", fA()))
	if before != "XXabXX" {
		t.Fatalf("fmt.Appendf with an observing later call = %q, want %q", before, "XXabXX")
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

// TestEquiv_PS2033_AfterAllocProfile pins the perf premise: into existing
// capacity the chain allocates nothing, while fmt.Appendf boxes each string
// operand into an interface. The operands live at package scope (as in the
// benchmark) so the compiler cannot prove the boxes don't escape.
var (
	ps2033AllocBuf = make([]byte, 0, 256)
	ps2033AllocA   = "hello, "
	ps2033AllocB   = "world"
	ps2033AllocOut []byte
)

func TestEquiv_PS2033_AfterAllocProfile(t *testing.T) {
	if avg := testing.AllocsPerRun(200, func() {
		ps2033AllocOut = ps2033After2(ps2033AllocBuf[:0], ps2033AllocA, ps2033AllocB)
	}); avg != 0 {
		t.Errorf("append chain into existing capacity allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(200, func() {
		ps2033AllocOut = ps2033Before2(ps2033AllocBuf[:0], ps2033AllocA, ps2033AllocB)
	}); avg < 1 {
		t.Logf("fmt.Appendf allocates %v times per run — the compiler learned to elide the boxes; the win claim may need re-framing", avg)
	}
}
