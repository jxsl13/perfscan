package checks

// Runtime differential for PS2034: fmt.Sprintf("host=%s;port=%s", a, b) over
// plain strings versus the direct concatenation "host=" + a + ";port=" + b.
// The fix's safety argument is that a bare %s (no flag/width/precision) over a
// value of EXACTLY the predeclared string type emits that string's bytes
// verbatim (invalid UTF-8 and empties included; a string is never nil), and
// fmt emits every literal segment of the format verbatim, so the whole result
// is byte-for-byte the concatenation of segments and operands. This suite
// pins:
//
//   - byte identity for the fixed shapes (leading/mid/trailing segments,
//     empty segments between adjacent verbs, escape-heavy and non-ASCII
//     segments) over adversarial operands (empty, NUL, invalid UTF-8, a lone
//     surrogate, 4 KiB, random byte strings with a fixed seed);
//   - a fixer simulation: for every format ps2034Split accepts, the
//     rewrite's runtime value — the segments (round-tripped through
//     strconv.Quote/Unquote exactly as the fix re-emits them) concatenated
//     with the operands — equals fmt.Sprintf's output, over randomized
//     formats and operands;
//   - the splitter's rejections: %%, %d, flags, widths, precision, a
//     trailing %, all of which genuinely diverge from (or fall outside) the
//     splice shape;
//   - evaluation order and count parity: both forms run each operand
//     expression once, left to right;
//   - the divergence the plain-string gate excludes, reproduced positively:
//     a named string type whose String() method %s honors and + would not;
//   - the perf premise: the + chain makes exactly one allocation while
//     fmt.Sprintf boxes each operand and round-trips its pp buffer.

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
)

func ps2034BeforeHostPort(a, b string) string { return fmt.Sprintf("host=%s;port=%s", a, b) }
func ps2034AfterHostPort(a, b string) string  { return "host=" + a + ";port=" + b }

func ps2034BeforeLead(a string) string { return fmt.Sprintf("user=%s", a) }
func ps2034AfterLead(a string) string  { return "user=" + a }

func ps2034BeforeTrail(a string) string { return fmt.Sprintf("%s!", a) }
func ps2034AfterTrail(a string) string  { return a + "!" }

func ps2034BeforeMixed(a, b, c string) string { return fmt.Sprintf("[%s:%s%s]", a, b, c) }
func ps2034AfterMixed(a, b, c string) string  { return "[" + a + ":" + b + c + "]" }

func ps2034BeforeEscapes(a string) string { return fmt.Sprintf("a\tb%s\n\x00é世", a) }
func ps2034AfterEscapes(a string) string  { return "a\tb" + a + "\n\x00é世" }

func ps2034Operands() []string {
	ops := []string{
		"", "a", " ", "hello",
		"\x00", "a\x00b",
		"\xff\xfe\x80",       // invalid UTF-8 — %s and + both carry the raw bytes
		string(rune(0xD800)), // lone surrogate literal encodes as RuneError
		"héllo, 世界", "\U0001F600",
		"%s", "%d", "100%", // percent signs in OPERANDS are data for both forms
		strings.Repeat("q", 4096),
	}
	rng := rand.New(rand.NewSource(0x2034))
	for range 500 {
		b := make([]byte, rng.Intn(48))
		rng.Read(b)
		ops = append(ops, string(b))
	}
	return ops
}

func TestEquiv_PS2034_ByteIdentity(t *testing.T) {
	ops := ps2034Operands()
	rng := rand.New(rand.NewSource(1))
	pick := func() string { return ops[rng.Intn(len(ops))] }
	check := func(before, after, label string) {
		t.Helper()
		if before != after {
			t.Fatalf("%s diverges: before=%q after=%q", label, before, after)
		}
	}
	// The full adversarial cross for the two-operand shape, plus randomized
	// runs over every fixed shape.
	for _, a := range ops[:14] {
		for _, b := range ops[:14] {
			check(ps2034BeforeHostPort(a, b), ps2034AfterHostPort(a, b), fmt.Sprintf("host-port (%q,%q)", a, b))
		}
	}
	for range 3000 {
		a, b, c := pick(), pick(), pick()
		check(ps2034BeforeHostPort(a, b), ps2034AfterHostPort(a, b), "host-port random")
		check(ps2034BeforeLead(a), ps2034AfterLead(a), "lead random")
		check(ps2034BeforeTrail(a), ps2034AfterTrail(a), "trail random")
		check(ps2034BeforeMixed(a, b, c), ps2034AfterMixed(a, b, c), "mixed random")
		check(ps2034BeforeEscapes(a), ps2034AfterEscapes(a), "escapes random")
	}
}

// TestEquiv_PS2034_FixerSimulation drives the ACTUAL splitter the check uses:
// for every format ps2034Split accepts, the rewrite's runtime value is the
// concatenation of the literal segments and the operands in order — with each
// segment round-tripped through strconv.Quote + strconv.Unquote, exactly the
// literal the fix emits — and must equal fmt.Sprintf's output byte-for-byte.
// Formats are both a hand-picked adversarial set and randomized interleavings
// of arbitrary segment bytes (percent-free) with %s verbs.
func TestEquiv_PS2034_FixerSimulation(t *testing.T) {
	ops := ps2034Operands()
	rng := rand.New(rand.NewSource(2))
	pick := func() string { return ops[rng.Intn(len(ops))] }

	simulate := func(format string) {
		t.Helper()
		segs, ok := ps2034Split(format)
		if !ok {
			t.Fatalf("ps2034Split rejected %q, expected accept", format)
		}
		k := len(segs) - 1
		args := make([]string, k)
		boxed := make([]any, k)
		for i := range args {
			args[i] = pick()
			boxed[i] = args[i]
		}
		before := fmt.Sprintf(format, boxed...)
		var sb strings.Builder
		for i, seg := range segs {
			// The fix emits strconv.Quote(seg); its runtime value is
			// strconv.Unquote of that literal. Pin the round-trip too.
			quoted := strconv.Quote(seg)
			decoded, err := strconv.Unquote(quoted)
			if err != nil || decoded != seg {
				t.Fatalf("strconv.Quote(%q) does not round-trip: %q, err=%v", seg, quoted, err)
			}
			sb.WriteString(decoded)
			if i < k {
				sb.WriteString(args[i])
			}
		}
		if after := sb.String(); before != after {
			t.Fatalf("format %q diverges over %q: before=%q after=%q", format, args, before, after)
		}
	}

	for _, format := range []string{
		"host=%s;port=%s", "user=%s", "%s!", "[%s:%s%s]",
		"a\tb%s\n", "\x00%s\xff", "héllo %s 世界",
		"%s-%s-%s-%s", " %s ", "=%s",
	} {
		simulate(format)
	}
	// Randomized formats: 1..4 verbs with random percent-free segments
	// (possibly empty, possibly non-UTF-8) around them, at least one
	// segment forced non-empty (the check's hasText gate).
	segBytes := func() string {
		b := make([]byte, rng.Intn(6))
		rng.Read(b)
		s := strings.ReplaceAll(string(b), "%", "@")
		return s
	}
	for range 2000 {
		k := 1 + rng.Intn(4)
		var f strings.Builder
		f.WriteString(segBytes())
		for range k {
			f.WriteString("%s")
			f.WriteString(segBytes())
		}
		format := f.String()
		if strings.ReplaceAll(format, "%s", "") == "" {
			format = "x" + format // force a non-empty segment
		}
		simulate(format)
	}

	// The splitter must reject everything outside the bare-%s splice shape.
	for _, format := range []string{
		"100%%%s", "%d:%s", "v=%5s", "v=%.3s", "v=%-s", "v=%s%",
		"%v", "%q%s", "%", "a%", "%\x00s", "% s",
	} {
		if _, ok := ps2034Split(format); ok {
			t.Errorf("ps2034Split accepted %q, expected reject", format)
		}
	}
	// And accept pure-verb formats structurally (the run gate, not the
	// splitter, routes those to PS2122/PS2130 via hasText).
	if segs, ok := ps2034Split("%s%s"); !ok || len(segs) != 3 || segs[0] != "" || segs[1] != "" || segs[2] != "" {
		t.Errorf("ps2034Split(%%s%%s) = %q, %v — want three empty segments", segs, ok)
	}
}

// TestEquiv_PS2034_EvaluationOrder pins that both forms run each operand
// expression exactly once, left to right.
func TestEquiv_PS2034_EvaluationOrder(t *testing.T) {
	var trace []string
	aFn := func() string { trace = append(trace, "a"); return "aa" }
	bFn := func() string { trace = append(trace, "b"); return "bb" }

	trace = nil
	before := fmt.Sprintf("x=%s y=%s", aFn(), bFn())
	beforeTrace := strings.Join(trace, ",")

	trace = nil
	after := "x=" + aFn() + " y=" + bFn()
	afterTrace := strings.Join(trace, ",")

	if beforeTrace != afterTrace {
		t.Fatalf("evaluation order diverges: before=%s after=%s", beforeTrace, afterTrace)
	}
	if beforeTrace != "a,b" {
		t.Fatalf("unexpected evaluation trace %q, want a,b", beforeTrace)
	}
	if before != after || after != "x=aa y=bb" {
		t.Fatalf("results diverge: before=%q after=%q", before, after)
	}
}

// ps2034Named reproduces the divergence the plain-string gate excludes: %s on
// a NAMED string type honors its String() method, + does not.
type ps2034Named string

func (n ps2034Named) String() string { return "Mx. " + string(n) }

func TestEquiv_PS2034_NamedStringDivergence(t *testing.T) {
	n := ps2034Named("kim")
	before := fmt.Sprintf("who=%s", n)
	if before != "who=Mx. kim" {
		t.Fatalf("fmt.Sprintf over the named string = %q, want %q", before, "who=Mx. kim")
	}
	after := "who=" + string(n)
	if after != "who=kim" {
		t.Fatalf("concatenation over the named string = %q, want %q", after, "who=kim")
	}
	if before == after {
		t.Fatal("expected the named-string shapes to diverge — the plain-string gate would be unnecessary")
	}
}

// TestEquiv_PS2034_AfterAllocProfile pins the perf premise: the + chain makes
// exactly one allocation (the result) while fmt.Sprintf boxes each operand and
// round-trips its pp buffer. The operands live at package scope so the
// compiler cannot constant-fold or elide anything.
var (
	ps2034AllocA   = "h.example.internal"
	ps2034AllocB   = "58080"
	ps2034AllocOut string
)

func TestEquiv_PS2034_AfterAllocProfile(t *testing.T) {
	if avg := testing.AllocsPerRun(200, func() {
		ps2034AllocOut = ps2034AfterHostPort(ps2034AllocA, ps2034AllocB)
	}); avg > 1 {
		t.Errorf("+ chain allocates %v times per run, want at most 1 (the result)", avg)
	}
	if avg := testing.AllocsPerRun(200, func() {
		ps2034AllocOut = ps2034BeforeHostPort(ps2034AllocA, ps2034AllocB)
	}); avg <= 1 {
		t.Logf("fmt.Sprintf allocates %v times per run — the compiler learned to elide the boxes; the win claim may need re-framing", avg)
	}
}
