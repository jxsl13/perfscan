package ps3101

import "bytes"

func use(bool) {}

func sink([]byte) {}

func useStr(string) {}

func invariantConv(lines [][]byte, sep string) {
	for _, line := range lines {
		use(bytes.Contains(line, []byte(sep))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

func stringConv(bufs [][]byte, key []byte) map[string]bool {
	seen := map[string]bool{}
	for range bufs {
		seen[string(key)] = true // want `string\(key\) copies its operand on every iteration but key is loop-invariant; hoist the conversion above the loop`
	}
	return seen
}

// Invariant across both loops: the fix hoists before the OUTERMOST loop.
func nestedInvariant(lines [][]byte, sep string) {
	for i := 0; i < 3; i++ {
		for _, line := range lines {
			use(bytes.Contains(line, []byte(sep))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
		}
	}
}

// Invariant for the INNER loop but iterated by the outer one: advisory only,
// no fix — hoisting before the outer loop would freeze the first sep.
func outerLoopVar(lines [][]byte, seps []string) {
	for _, sep := range seps {
		for _, line := range lines {
			use(bytes.Contains(line, []byte(sep))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
		}
	}
}

// Reassigned in the outer loop's body only: advisory only, no fix.
func outerReassign(lines [][]byte, sep string) {
	for i := 0; i < 3; i++ {
		for _, line := range lines {
			use(bytes.Contains(line, []byte(sep))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
		}
		sep = "y"
	}
}

// The []byte result escapes to an unknown function that could mutate or
// retain it: advisory only, no fix.
func escapes(lines [][]byte, sep string) {
	for range lines {
		sink([]byte(sep)) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

// Bound to a variable and written through: sharing one slice across
// iterations would change behavior, advisory only, no fix.
func rebound(lines [][]byte, sep string) {
	for range lines {
		b := []byte(sep) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
		b[0] = 'x'
	}
}

// Redeclared inside the loop body: hoisting would read a different (or
// undefined) variable, advisory only, no fix.
func shadowDecl(bufs [][]byte) {
	for range bufs {
		var key []byte
		useStr(string(key)) // want `string\(key\) copies its operand on every iteration but key is loop-invariant; hoist the conversion above the loop`
	}
}

type raw []byte

// Conversion to a named type whose underlying type is []byte: reported, but
// the fix only reproduces the canonical []byte/string spelling — no fix.
func named(lines [][]byte, sep string) {
	for range lines {
		useRaw(raw(sep)) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

func useRaw(raw) {}

// The operand is the loop variable: it changes per iteration, silent.
func perItemConv(lines []string) [][]byte {
	out := make([][]byte, 0, len(lines))
	for _, line := range lines {
		out = append(out, []byte(line))
	}
	return out
}

// Reassigned in the body: not invariant, silent.
func reassigned(lines [][]byte, sep string) {
	for _, line := range lines {
		use(bytes.Contains(line, []byte(sep)))
		sep = "y"
	}
}

func hoisted(lines [][]byte, sep string) {
	bsep := []byte(sep)
	for _, line := range lines {
		use(bytes.Contains(line, bsep))
	}
}

// The outermost loop is labeled: inserting a binding between the label and
// the loop would re-target the label, advisory only, no fix.
func labeled(lines [][]byte, sep string) {
outer:
	for _, line := range lines {
		use(bytes.Contains(line, []byte(sep))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
		if len(line) == 0 {
			continue outer
		}
	}
}

// Regression (go/doc/comment inc(), caught by the stdlib validation run):
// the loop mutates b's ELEMENTS, so a hoisted string(b) would freeze the
// pre-mutation bytes. string(X) with a slice-typed operand is never
// hoisted.
func incShape(s string) string {
	b := []byte(s)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < '9' {
			b[i]++
			return string(b) // want `string\(b\) copies its operand on every iteration but b is loop-invariant; hoist the conversion above the loop`
		}
		b[i] = '0'
	}
	return "1" + string(b)
}
