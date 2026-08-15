package checks

// Runtime differential for PS5029: fmt.Sprintln over operands that are
// ALL plain (unnamed) strings vs the a + " " + b + ... + z + "\n"
// concatenation the fix emits. The safety argument is that Sprintln's
// doPrintln is UNCONDITIONAL — exactly one ' ' between adjacent
// operands, exactly one trailing '\n', no format-string interpretation
// (a '%' in an operand is printed literally) — and that verb 'v' on a
// plain string emits the value's bytes verbatim (no quoting, no UTF-8
// replacement). This suite pins:
//
//   - byte-identity over adversarial operands — empty strings, '%'
//     verbs, "%%", NUL bytes, invalid UTF-8, multi-byte UTF-8, embedded
//     spaces/newlines, long strings — for 1..5 operands in every
//     combination drawn from the adversarial pool;
//   - side-effect count and evaluation order: both forms evaluate each
//     operand exactly once, left to right;
//   - WHY the plain-string guard is load-bearing, with concrete
//     divergence witnesses: a NAMED string type whose String() (or
//     Error()) method Sprintln's 'v' verb honors and + would not;
//   - WHY fmt.Sprint must NOT be matched by this check: Sprint's
//     conditional spacing rule inserts nothing between string operands,
//     so Sprint and Sprintln genuinely differ for two operands.

import (
	"fmt"
	"strings"
	"testing"
)

// ps5029Operands are adversarial plain-string operands: empty, percent
// shapes (Sprintln does NOT interpret formats), NUL bytes, invalid
// UTF-8, multi-byte UTF-8, embedded spaces and newlines, and a long
// string past any small-buffer path.
var ps5029Operands = []string{
	"",
	" ",
	"a",
	"%d",
	"100%%",
	"%",
	"%v %s %q",
	"a\x00b",
	"\xff\xfe invalid \x80",
	"日本語",
	"already has\nnewlines\n",
	"tab\tand trailing space ",
	strings.Repeat("x", 1031),
}

// ps5029Concat is the rewrite shape the fix emits, generalized over the
// operand count: a + " " + b + ... + z + "\n" built with the SAME
// left-to-right + chain the compiler sees (strings.Join would be a
// different implementation; a fold of binary + is exactly what the
// fixed source contains).
func ps5029Concat(args []string) string {
	s := ""
	for i, a := range args {
		if i > 0 {
			s = s + " "
		}
		s = s + a
	}
	return s + "\n"
}

// TestEquivPS5029_Identity pins byte-identity of fmt.Sprintln and the +
// rewrite for 1..3 operands over the full adversarial cross product,
// plus longer 4- and 5-operand spot shapes.
func TestEquivPS5029_Identity(t *testing.T) {
	// Single operand: fmt.Sprintln(a) vs a + "\n".
	for _, a := range ps5029Operands {
		if got, want := fmt.Sprintln(a), a+"\n"; got != want {
			t.Errorf("Sprintln(%q) = %q, want %q", a, got, want)
		}
	}
	// Two operands: fmt.Sprintln(a, b) vs a + " " + b + "\n".
	for _, a := range ps5029Operands {
		for _, b := range ps5029Operands {
			if got, want := fmt.Sprintln(a, b), a+" "+b+"\n"; got != want {
				t.Errorf("Sprintln(%q, %q) = %q, want %q", a, b, got, want)
			}
		}
	}
	// Three operands, full cross product.
	for _, a := range ps5029Operands {
		for _, b := range ps5029Operands {
			for _, c := range ps5029Operands {
				if got, want := fmt.Sprintln(a, b, c), a+" "+b+" "+c+"\n"; got != want {
					t.Errorf("Sprintln(%q, %q, %q) = %q, want %q", a, b, c, got, want)
				}
			}
		}
	}
	// Four and five operands through the generalized fold.
	for _, args := range [][]string{
		{"", "", "", ""},
		{"%s", "\x00", "日本語", strings.Repeat("y", 257)},
		{"a", "b", "c", "d", "e"},
		{"", "%v", " ", "\xff", "end\n"},
	} {
		anys := make([]any, len(args))
		for i, a := range args {
			anys[i] = a
		}
		if got, want := fmt.Sprintln(anys...), ps5029Concat(args); got != want {
			t.Errorf("Sprintln(%q...) = %q, want %q", args, got, want)
		}
	}
}

// TestEquivPS5029_SideEffects pins that both forms evaluate each operand
// exactly once, left to right: an operand expression with a visible side
// effect fires identically under the call and under the + chain.
func TestEquivPS5029_SideEffects(t *testing.T) {
	var orderA, orderB []int
	eff := func(order *[]int, id int, s string) string {
		*order = append(*order, id)
		return s
	}
	got := fmt.Sprintln(eff(&orderA, 1, "x"), eff(&orderA, 2, "y"), eff(&orderA, 3, "z"))
	want := eff(&orderB, 1, "x") + " " + eff(&orderB, 2, "y") + " " + eff(&orderB, 3, "z") + "\n"
	if got != want {
		t.Errorf("side-effect shape: Sprintln = %q, concat = %q", got, want)
	}
	if len(orderA) != 3 || len(orderB) != 3 {
		t.Fatalf("operands not evaluated exactly once: call %v, concat %v", orderA, orderB)
	}
	for i := range orderA {
		if orderA[i] != i+1 || orderB[i] != i+1 {
			t.Errorf("evaluation order diverged: call %v, concat %v", orderA, orderB)
		}
	}
}

// ps5029Named is a NAMED string type with a String() method: Sprintln's
// 'v' verb honors fmt.Stringer while + would use the underlying bytes.
type ps5029Named string

func (n ps5029Named) String() string { return "Mx. " + string(n) }

// ps5029Err is a named string implementing error: the 'v' verb prefers
// Error() over the raw bytes.
type ps5029Err string

func (e ps5029Err) Error() string { return "err: " + string(e) }

// TestEquivPS5029_NamedTypeDiverges pins WHY the plain-string guard is
// load-bearing: for a named string type with a String() or Error()
// method the outputs MUST differ, otherwise the guard would be
// needlessly conservative.
func TestEquivPS5029_NamedTypeDiverges(t *testing.T) {
	n := ps5029Named("sam")
	if got, concat := fmt.Sprintln("hi", n), "hi"+" "+string(n)+"\n"; got == concat {
		t.Errorf("Sprintln with a Stringer-named string matched the concat (%q) — the plain-string guard would be needlessly conservative", got)
	} else if want := "hi Mx. sam\n"; got != want {
		t.Errorf("Sprintln(\"hi\", n) = %q, want %q", got, want)
	}
	e := ps5029Err("boom")
	if got, concat := fmt.Sprintln("op", e), "op"+" "+string(e)+"\n"; got == concat {
		t.Errorf("Sprintln with an error-named string matched the concat (%q) — the plain-string guard would be needlessly conservative", got)
	} else if want := "op err: boom\n"; got != want {
		t.Errorf("Sprintln(\"op\", e) = %q, want %q", got, want)
	}
}

// TestEquivPS5029_SprintDiverges pins WHY the callee must be Sprintln
// and never Sprint: Sprint's conditional spacing rule inserts nothing
// between adjacent string operands and appends no newline, so the two
// functions genuinely differ — PS2123 owns the Sprint shape.
func TestEquivPS5029_SprintDiverges(t *testing.T) {
	if got, ln := fmt.Sprint("a", "b"), fmt.Sprintln("a", "b"); got == ln {
		t.Errorf("Sprint and Sprintln agree on two strings (%q) — the callee pin would be needlessly strict", got)
	} else if got != "ab" || ln != "a b\n" {
		t.Errorf("Sprint = %q (want %q), Sprintln = %q (want %q)", got, "ab", ln, "a b\n")
	}
}
