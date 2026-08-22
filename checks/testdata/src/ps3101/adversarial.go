package ps3101

import (
	"bytes"
	"strings"
)

// Adversarial []byte-aliasing negatives: every case below must stay advisory
// (or silent) — a hoist would share one mutable buffer across iterations with
// a consumer that mutates, retains, or aliases it, or would freeze an operand
// that actually changes between iterations. golden == source.

// bytes.Split returns subslices ALIASING its arguments: not read-only, no fix.
func advSplit(lines [][]byte, sep string) {
	for _, line := range lines {
		_ = bytes.Split(line, []byte(sep)) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

// bytes.Cut returns aliases of its first argument: not read-only, no fix.
func advCut(lines [][]byte, sep string) {
	for _, line := range lines {
		_, _, _ = bytes.Cut(line, []byte(sep)) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

// bytes.Replace is not a pure predicate: no fix.
func advReplace(lines [][]byte, old string) {
	for _, line := range lines {
		_ = bytes.Replace(line, []byte(old), nil, -1) // want `\[\]byte\(old\) copies its operand on every iteration but old is loop-invariant; hoist the conversion above the loop`
	}
}

// bytes.Join reads sep but is not in the read-only predicate set: no fix.
func advJoin(chunks [][]byte, sep string) {
	for range chunks {
		_ = bytes.Join(chunks, []byte(sep)) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

// bytes.NewReader RETAINS its argument: no fix.
func advNewReader(lines [][]byte, sep string) {
	for range lines {
		_ = bytes.NewReader([]byte(sep)) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

// append(dst, conv...) copies out of the conversion, but append is not a
// bytes.* predicate and dst retains nothing provable: no fix.
func advAppendSpread(lines [][]byte, sep string) []byte {
	var dst []byte
	for range lines {
		dst = append(dst, []byte(sep)...) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
	return dst
}

// The conversion is parenthesized, so its direct parent is not the bytes.*
// call: conservatively no fix.
func advParen(lines [][]byte, sep string) {
	for _, line := range lines {
		use(bytes.Contains(line, ([]byte(sep)))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

// The conversion is sliced before being passed: its direct parent is the
// slice expression, not the read-only call: no fix.
func advSliced(lines [][]byte, sep string) {
	for _, line := range lines {
		use(bytes.Contains(line, []byte(sep)[1:])) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

// The conversion is an argument of append, which mutates its result: no fix.
func advAppendArg(lines [][]byte, sep string) {
	for range lines {
		_ = append([]byte(sep), 'x') // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

// string(b) consumed read-only is STILL never hoisted: the operand is a
// mutable slice and a snapshot could freeze stale bytes.
func advStringReadOnly(lines [][]byte, b []byte, s string) {
	for range lines {
		use(strings.Contains(string(b), s)) // want `string\(b\) copies its operand on every iteration but b is loop-invariant; hoist the conversion above the loop`
	}
}

// sep is reassigned in the for-loop's POST statement — it changes every
// iteration even though the body never mentions it. Silent: not invariant.
func advPostMutation(lines [][]byte, sep, next string) {
	for i := 0; i < len(lines); i, sep = i+1, next {
		use(bytes.Contains(lines[i], []byte(sep)))
	}
}

// sep is mutated by a closure invoked from the loop's COND: the operand
// changes between iterations, so no fix may be attached.
func advCondClosure(lines [][]byte, sep string) {
	i := 0
	bump := func() bool { sep += "x"; return i < len(lines) }
	for bump() {
		use(bytes.Contains(lines[i], []byte(sep))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
		i++
	}
}

// sep is mutated by a closure declared BEFORE the loop and called from the
// body: the assignment is lexically outside the loop statement, no fix.
func advBodyClosureCall(lines [][]byte, sep string) {
	g := func() { sep = "zzz" }
	for _, line := range lines {
		use(bytes.Contains(line, []byte(sep))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
		g()
	}
}

var pkgSep = "start"

func bumpPkgSep() { pkgSep = "changed" }

// The operand is a package-level VARIABLE: any callee may reassign it between
// iterations, so no fix.
func advPkgLevelOperand(lines [][]byte) {
	for _, line := range lines {
		use(bytes.Contains(line, []byte(pkgSep))) // want `\[\]byte\(pkgSep\) copies its operand on every iteration but pkgSep is loop-invariant; hoist the conversion above the loop`
		bumpPkgSep()
	}
}

func mutateVia(p *string) { *p = "mut" }

// The operand's address is taken before the loop and a callee mutates it via
// the pointer: no fix.
func advAddrTaken(lines [][]byte, sep string) {
	p := &sep
	for _, line := range lines {
		use(bytes.Contains(line, []byte(sep))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
		mutateVia(p)
	}
}

// The conversion sits inside a closure inside the loop: a function literal is
// an iteration boundary, entirely silent.
func advClosureInLoop(lines [][]byte, sep string) {
	for _, line := range lines {
		f := func() bool { return bytes.Contains(line, []byte(sep)) }
		use(f())
	}
}
