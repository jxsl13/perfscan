package ps3101

import (
	"bytes"
	bs "bytes"
)

// Positive hoist cases proving the guard is not over-broad: each conversion
// is a direct argument of a read-only stdlib bytes predicate and the operand
// provably cannot change between iterations.

// A local CONSTANT operand is immutable: always safe to hoist.
func fixConstOperand(lines [][]byte) {
	const csep = ","
	for _, line := range lines {
		use(bytes.Contains(line, []byte(csep))) // want `\[\]byte\(csep\) copies its operand on every iteration but csep is loop-invariant; hoist the conversion above the loop`
	}
}

// The stdlib package resolved through an IMPORT ALIAS is still the stdlib
// package: the guard is type-resolved, not spelled-name-based.
func fixAliasImport(lines [][]byte, sep string) {
	for _, line := range lines {
		use(bs.Contains(line, []byte(sep))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

// Two independent read-only conversions of DIFFERENT operands in sibling
// loops: each gets its own hoisted binding.
func fixTwoLoops(lines [][]byte, pre, suf string) {
	for _, line := range lines {
		use(bytes.HasPrefix(line, []byte(pre))) // want `\[\]byte\(pre\) copies its operand on every iteration but pre is loop-invariant; hoist the conversion above the loop`
	}
	for _, line := range lines {
		use(bytes.HasSuffix(line, []byte(suf))) // want `\[\]byte\(suf\) copies its operand on every iteration but suf is loop-invariant; hoist the conversion above the loop`
	}
}

// Both arguments of a read-only predicate are invariant conversions: both are
// hoisted, sharing two distinct read-only buffers.
func fixBothArgs(n int, a, b string) {
	for i := 0; i < n; i++ {
		use(bytes.Equal([]byte(a), []byte(b))) // want `\[\]byte\(a\) copies its operand on every iteration but a is loop-invariant; hoist the conversion above the loop` `\[\]byte\(b\) copies its operand on every iteration but b is loop-invariant; hoist the conversion above the loop`
	}
}
