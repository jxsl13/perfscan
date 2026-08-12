package ps2128

import (
	"fmt"
)

// acc := "" form, plain counted for loop: fixed, and this first fix
// carries the added strings import for the file. EMPTY init: no seed
// WriteString is emitted.
func joinDefine(items []string) string {
	acc := "" // want `acc is grown by string concatenation on every loop iteration`
	for i := 0; i < len(items); i++ {
		acc += items[i]
	}
	return acc
}

// var acc string form over a range loop: fixed.
func joinVar(items []string) string {
	var acc string // want `acc is grown by string concatenation on every loop iteration`
	for _, it := range items {
		acc += it
	}
	return acc
}

// acc = acc + x spelling of the append: fixed.
func joinBinary(items []string) string {
	out := "" // want `out is grown by string concatenation on every loop iteration`
	for _, it := range items {
		out = out + it
	}
	return out
}

// Append guarded by an if inside the loop, and two post-loop reads: both
// reads become String() calls.
func joinGuarded(items []string) (string, int) {
	acc := "" // want `acc is grown by string concatenation on every loop iteration`
	for _, it := range items {
		if it != "" {
			acc += it
		}
	}
	return acc, len(acc)
}

// The name strings is shadowed at the rewrite sites: reported, no fix.
func joinShadowed(items []string) string {
	strings := 0
	_ = strings
	acc := "" // want `acc is grown by string concatenation on every loop iteration`
	for _, it := range items {
		acc += it
	}
	return acc
}

// NEGATIVE: acc is read inside the loop (the separator idiom) — changing
// the type would touch that read too; silent.
func readInLoop(items []string) string {
	acc := ""
	for _, it := range items {
		if len(acc) > 0 {
			acc += ","
		}
		acc += it
	}
	return acc
}

// NEGATIVE: acc is captured by a closure; silent.
func closureCapture(items []string) string {
	acc := ""
	for _, it := range items {
		acc += it
	}
	f := func() string { return acc }
	return f()
}

// NEGATIVE: acc has its address taken; silent.
func addressTaken(items []string) *string {
	acc := ""
	for _, it := range items {
		acc += it
	}
	return &acc
}

// NEGATIVE: acc is reassigned after the loop; silent.
func reassignedAfter(items []string) string {
	acc := ""
	for _, it := range items {
		acc += it
	}
	acc = acc + "!"
	return acc
}

// NEGATIVE: a statement sits between the declaration and the loop; silent.
func statementBetween(items []string) string {
	acc := ""
	n := len(items)
	for i := 0; i < n; i++ {
		acc += items[i]
	}
	return acc
}

// SEEDED: the accumulator starts non-empty — the seed is preserved via a
// leading WriteString, still evaluated at the declaration point: fixed.
func seeded(items []string) string {
	acc := "prefix:" // want `acc is grown by string concatenation on every loop iteration`
	for _, it := range items {
		acc += it
	}
	return acc
}

// NEGATIVE: the appended expression references the accumulator; silent.
func selfAppend(items []string) string {
	acc := ""
	for range items {
		acc += acc
	}
	return acc
}

// NEGATIVE: not the predeclared string spelling (an alias); silent.
type str = string

func aliasedType(items []string) str {
	var acc str
	for _, it := range items {
		acc += it
	}
	return acc
}

// NEGATIVE: not a string at all; silent.
func sumInts(xs []int) int {
	acc := 0
	for _, x := range xs {
		acc += x
	}
	return acc
}

// NEGATIVE: the accumulator is never read after the loop; silent.
func neverRead(items []string) {
	acc := ""
	for _, it := range items {
		acc += it
	}
}

// SEEDED: var acc = E form (type inferred string): fixed, seed preserved.
func seededVar(items []string) string {
	var acc = "x" // want `acc is grown by string concatenation on every loop iteration`
	for _, it := range items {
		acc += it
	}
	return acc
}

// SEEDED: var acc string = fmt.Sprintf(...) — the seed is a call; it is
// still evaluated exactly once, at the declaration point, before the loop.
func seededSprintf(items []string, n int) string {
	var acc string = fmt.Sprintf("%d:", n) // want `acc is grown by string concatenation on every loop iteration`
	for _, it := range items {
		acc += it
	}
	return acc
}

// SEEDED with an if-guarded append inside the loop: fixed.
func seededGuarded(items []string) string {
	acc := "start:" // want `acc is grown by string concatenation on every loop iteration`
	for _, it := range items {
		if it != "" {
			acc += it
		}
	}
	return acc
}

// NEGATIVE (report-only): the seed's source range contains a comment the
// two-line rewrite cannot place; reported, no fix.
func seededComment(items []string) string {
	acc := "a" + /* seed */ "b" // want `acc is grown by string concatenation on every loop iteration`
	for _, it := range items {
		acc += it
	}
	return acc
}

// Seed references an OUTER binding of the same name: the inserted
// `var acc strings.Builder` would shadow it and rebind the seed's acc to the
// Builder (a type error). Must not fire (regression: adversarial c05).
func seededShadow(acc string, ws []string) string {
	if len(ws) > 0 {
		acc := acc + "-"
		for _, w := range ws {
			acc += w
		}
		return acc
	}
	return acc
}

// A parenthesized empty initializer is still the seedless empty path — no
// WriteString("") is emitted (regression: adversarial c11/c21).
func parenEmpty(ws []string) string {
	acc := ("") // want `acc is grown by string concatenation on every loop iteration`
	for _, w := range ws {
		acc += w
	}
	return acc
}
