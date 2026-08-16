package checks

// Runtime differential for PS2044: fmt.Appendf(buf, "%s=%s;", k, v) — literal
// text spliced with bare %s verbs over plain strings — versus the nested chain
// append(append(append(append(buf, k...), "="...), v...), ";"...). The fix's
// safety argument is that %s with no flag/width over a value of EXACTLY the
// predeclared string type writes that string's bytes verbatim (invalid UTF-8
// and empties included), literal format text is written verbatim too, and
// fmt.Appendf then appends the formatted buffer to buf — exactly the bytes the
// builtin append chain writes in the same interleaved order. This suite pins:
//
//   - byte identity, result length and nil-ness parity for the leading,
//     middle and trailing literal-segment placements (1- and 2-operand
//     shapes) over adversarial operands (empty, NUL, invalid UTF-8, a lone
//     surrogate, 4 KiB, random byte strings with a fixed seed) crossed with
//     adversarial destinations (nil, non-nil empty, seeded, spare capacity,
//     zero capacity) and adversarial literal segments (escapes, non-ASCII,
//     NUL, invalid UTF-8);
//   - evaluation order and count parity for the shapes the fix accepts (an
//     arbitrary buf expression and — with an EMPTY leading segment — an
//     arbitrary FIRST operand, both calls);
//   - the three divergences the fix's gates exclude, reproduced positively:
//     a []byte operand aliasing buf's spare capacity (excluded by the
//     strings-only operand gate), a later operand call observing buf's array
//     between the chain's writes (excluded by the inert gate), and — the gate
//     unique to this check — a FIRST operand call observing buf's array after
//     the chain wrote a NON-EMPTY leading literal segment;
//   - the perf premise: the After shape is allocation-free into existing
//     capacity while fmt.Appendf boxes each operand.

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func ps2044BeforeMid(buf []byte, k, v string) []byte {
	return fmt.Appendf(buf, "%s=%s;", k, v)
}

func ps2044AfterMid(buf []byte, k, v string) []byte {
	return append(append(append(append(buf, k...), "="...), v...), ";"...)
}

func ps2044BeforeLead(buf []byte, h, p string) []byte {
	return fmt.Appendf(buf, "host=%s;port=%s", h, p)
}

func ps2044AfterLead(buf []byte, h, p string) []byte {
	return append(append(append(append(buf, "host="...), h...), ";port="...), p...)
}

func ps2044BeforeSingle(buf []byte, v string) []byte { return fmt.Appendf(buf, "k=%s", v) }
func ps2044AfterSingle(buf []byte, v string) []byte  { return append(append(buf, "k="...), v...) }

// Adversarial literal segments: the fix re-emits each segment with
// strconv.Quote of its decoded value, so escapes, NULs, invalid UTF-8 and
// non-ASCII text in the FORMAT must round-trip too.
func ps2044BeforeHairy(buf []byte, a string) []byte {
	return fmt.Appendf(buf, "\x00\xff…%s\n\t\"q\"", a)
}

func ps2044AfterHairy(buf []byte, a string) []byte {
	return append(append(append(buf, "\x00\xff…"...), a...), "\n\t\"q\""...)
}

func ps2044Bufs() [][]byte {
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

func TestEquiv_PS2044_ByteIdentity(t *testing.T) {
	ops := []string{
		"", "a", " ", "hello",
		"\x00", "a\x00b",
		"\xff\xfe\x80",       // invalid UTF-8 — %s and append both copy the raw bytes
		string(rune(0xD800)), // lone surrogate literal encodes as RuneError
		"héllo, 世界", "\U0001F600",
		strings.Repeat("q", 4096),
	}
	rng := rand.New(rand.NewSource(0x2044))
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
	// The full adversarial cross for the three segment placements, plus
	// randomized operands.
	for _, a := range ops[:11] {
		for _, b := range ops[:11] {
			for _, buf0 := range ps2044Bufs() {
				check(buf0,
					ps2044BeforeMid(append([]byte(nil), buf0...), a, b),
					ps2044AfterMid(append([]byte(nil), buf0...), a, b),
					fmt.Sprintf("mid (%q,%q)", a, b))
				check(buf0,
					ps2044BeforeLead(append([]byte(nil), buf0...), a, b),
					ps2044AfterLead(append([]byte(nil), buf0...), a, b),
					fmt.Sprintf("lead (%q,%q)", a, b))
			}
		}
		for _, buf0 := range ps2044Bufs() {
			check(buf0,
				ps2044BeforeSingle(append([]byte(nil), buf0...), a),
				ps2044AfterSingle(append([]byte(nil), buf0...), a),
				fmt.Sprintf("single (%q)", a))
			check(buf0,
				ps2044BeforeHairy(append([]byte(nil), buf0...), a),
				ps2044AfterHairy(append([]byte(nil), buf0...), a),
				fmt.Sprintf("hairy (%q)", a))
		}
	}
	for range 3000 {
		a, b := pick(), pick()
		for _, buf0 := range ps2044Bufs() {
			check(buf0,
				ps2044BeforeMid(append([]byte(nil), buf0...), a, b),
				ps2044AfterMid(append([]byte(nil), buf0...), a, b),
				"mid random")
			check(buf0,
				ps2044BeforeLead(append([]byte(nil), buf0...), a, b),
				ps2044AfterLead(append([]byte(nil), buf0...), a, b),
				"lead random")
			check(buf0,
				ps2044BeforeHairy(append([]byte(nil), buf0...), a),
				ps2044AfterHairy(append([]byte(nil), buf0...), a),
				"hairy random")
		}
	}
}

// TestEquiv_PS2044_EvaluationOrder pins that for the shapes the fix accepts —
// an arbitrary buf expression and, with an EMPTY leading segment, an arbitrary
// FIRST operand (both may be calls), later operands inert — both forms run the
// same user code in the same order, exactly once. The chain's first write to
// buf happens after both calls in both forms, so nothing can observe a
// difference.
func TestEquiv_PS2044_EvaluationOrder(t *testing.T) {
	var trace []string
	bufFn := func() []byte { trace = append(trace, "buf"); return make([]byte, 0, 8) }
	aFn := func() string { trace = append(trace, "a"); return "aa" }
	b := "bb"

	trace = nil
	before := fmt.Appendf(bufFn(), "%s=%s;", aFn(), b)
	beforeTrace := strings.Join(trace, ",")

	trace = nil
	after := append(append(append(append(bufFn(), aFn()...), "="...), b...), ";"...)
	afterTrace := strings.Join(trace, ",")

	if beforeTrace != afterTrace {
		t.Fatalf("evaluation order diverges: before=%s after=%s", beforeTrace, afterTrace)
	}
	if beforeTrace != "buf,a" {
		t.Fatalf("unexpected evaluation trace %q, want buf,a", beforeTrace)
	}
	if string(before) != string(after) || string(after) != "aa=bb;" {
		t.Fatalf("results diverge: before=%q after=%q", before, after)
	}
}

// TestEquiv_PS2044_ByteSliceOperandDivergence reproduces the divergence the
// strings-only operand gate excludes: a []byte operand whose bytes alias buf's
// spare capacity. fmt.Appendf reads every operand before writing to buf; the
// chain's earlier appends overwrite the aliased bytes before the later append
// reads them. Single-operand PS2141 can fix []byte operands (one read, one
// write, memmove-safe); the interleaved chain cannot.
func TestEquiv_PS2044_ByteSliceOperandDivergence(t *testing.T) {
	shape := func() ([]byte, []byte) {
		mem := make([]byte, 4, 16)
		copy(mem, "abcd")
		return mem[:2], mem[2:4] // buf and an operand aliasing its spare capacity
	}
	buf, bs := shape()
	before := fmt.Appendf(buf, "%s=%s", "XY", bs)
	if string(before) != "abXY=cd" {
		t.Fatalf("fmt.Appendf over aliasing []byte = %q, want %q", before, "abXY=cd")
	}
	buf, bs = shape()
	after := append(append(append(buf, "XY"...), "="...), bs...)
	if string(after) != "abXY=XY" {
		t.Fatalf("append chain over aliasing []byte = %q, want %q (the earlier appends clobber the operand)", after, "abXY=XY")
	}
	if string(before) == string(after) {
		t.Fatal("expected the aliasing []byte shapes to diverge — the operand gate would be unnecessary")
	}
}

// TestEquiv_PS2044_LaterCallDivergence reproduces the divergence the inert
// gate excludes: a LATER operand that runs user code observing buf's array.
// The chain writes into buf's capacity between operand evaluations;
// fmt.Appendf writes only after evaluating everything.
func TestEquiv_PS2044_LaterCallDivergence(t *testing.T) {
	arrA := make([]byte, 8)
	copy(arrA, "XXXXXXXX")
	fA := func() string { return string(arrA[3:5]) }
	before := string(fmt.Appendf(arrA[:2], "%s=%s", "a", fA()))
	if before != "XXa=XX" {
		t.Fatalf("fmt.Appendf with an observing later call = %q, want %q", before, "XXa=XX")
	}

	arrB := make([]byte, 8)
	copy(arrB, "XXXXXXXX")
	fB := func() string { return string(arrB[3:5]) }
	after := string(append(append(append(arrB[:2], "a"...), "="...), fB()...))
	if after != "XXa==X" {
		t.Fatalf("append chain with an observing later call = %q, want %q (the call sees the earlier writes)", after, "XXa==X")
	}
	if before == after {
		t.Fatal("expected the observing-call shapes to diverge — the inert gate would be unnecessary")
	}
}

// TestEquiv_PS2044_LeadingLiteralFirstCallDivergence reproduces the divergence
// unique to THIS check's gate (PS2033 never hits it, having no literal
// segments): with a NON-EMPTY leading segment the chain writes that constant
// into buf's capacity BEFORE evaluating the first operand, while fmt.Appendf
// evaluates every operand before writing anything. A first operand that runs
// user code observing buf's array sees the leading write — so with a leading
// literal even the FIRST operand must be inert.
func TestEquiv_PS2044_LeadingLiteralFirstCallDivergence(t *testing.T) {
	arrA := make([]byte, 8)
	copy(arrA, "XXXXXXXX")
	fA := func() string { return string(arrA[2:3]) }
	before := string(fmt.Appendf(arrA[:2], "L%s", fA()))
	if before != "XXLX" {
		t.Fatalf("fmt.Appendf with an observing first call = %q, want %q", before, "XXLX")
	}

	arrB := make([]byte, 8)
	copy(arrB, "XXXXXXXX")
	fB := func() string { return string(arrB[2:3]) }
	after := string(append(append(arrB[:2], "L"...), fB()...))
	if after != "XXLL" {
		t.Fatalf("append chain with an observing first call = %q, want %q (the call sees the leading-literal write)", after, "XXLL")
	}
	if before == after {
		t.Fatal("expected the leading-literal shapes to diverge — the all-operands-inert gate would be unnecessary")
	}
}

// TestEquiv_PS2044_AfterAllocProfile pins the perf premise: into existing
// capacity the chain allocates nothing, while fmt.Appendf boxes each string
// operand into an interface. The operands live at package scope (as in the
// benchmark) so the compiler cannot prove the boxes don't escape.
var (
	ps2044AllocBuf = make([]byte, 0, 256)
	ps2044AllocK   = "content-type"
	ps2044AllocV   = "text/plain"
	ps2044AllocOut []byte
)

func TestEquiv_PS2044_AfterAllocProfile(t *testing.T) {
	if avg := testing.AllocsPerRun(200, func() {
		ps2044AllocOut = ps2044AfterMid(ps2044AllocBuf[:0], ps2044AllocK, ps2044AllocV)
	}); avg != 0 {
		t.Errorf("append chain into existing capacity allocates %v times per run, want 0", avg)
	}
	if avg := testing.AllocsPerRun(200, func() {
		ps2044AllocOut = ps2044BeforeMid(ps2044AllocBuf[:0], ps2044AllocK, ps2044AllocV)
	}); avg < 1 {
		t.Logf("fmt.Appendf allocates %v times per run — the compiler learned to elide the boxes; the win claim may need re-framing", avg)
	}
}
