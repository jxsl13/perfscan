package ps2036

import (
	"fmt"
	"strconv"
)

// Keeps fmt and strconv referenced so the rewrites below neither orphan the
// fmt import nor need to add strconv (orphan.go and aliased.go exercise
// those paths).
func keep() { fmt.Println(strconv.Itoa(1)) }

// --- POSITIVES: one unnamed scalar operand ---

// int gets the value-preserving int64 widening.
func plainInt(buf []byte, n int) []byte {
	return fmt.Append(buf, n) // want `fmt\.Append with a single int/uint/bool/float operand`
}

// An int64 operand needs no widening wrapper.
func plainInt64(buf []byte, n int64) []byte {
	return fmt.Append(buf, n) // want `fmt\.Append with a single int/uint/bool/float operand`
}

func plainInt8(buf []byte, n int8) []byte {
	return fmt.Append(buf, n) // want `fmt\.Append with a single int/uint/bool/float operand`
}

func plainUint(buf []byte, u uint) []byte {
	return fmt.Append(buf, u) // want `fmt\.Append with a single int/uint/bool/float operand`
}

// A uint64 operand needs no widening wrapper.
func plainUint64(buf []byte, u uint64) []byte {
	return fmt.Append(buf, u) // want `fmt\.Append with a single int/uint/bool/float operand`
}

func plainUintptr(buf []byte, p uintptr) []byte {
	return fmt.Append(buf, p) // want `fmt\.Append with a single int/uint/bool/float operand`
}

func plainBool(buf []byte, b bool) []byte {
	return fmt.Append(buf, b) // want `fmt\.Append with a single int/uint/bool/float operand`
}

func plainFloat64(buf []byte, f float64) []byte {
	return fmt.Append(buf, f) // want `fmt\.Append with a single int/uint/bool/float operand`
}

// AppendFloat takes a float64: the operand is widened (value-preserving)
// while bitSize 32 keeps the float32 rounding.
func plainFloat32(buf []byte, f float32) []byte {
	return fmt.Append(buf, f) // want `fmt\.Append with a single int/uint/bool/float operand`
}

// The canonical buf = fmt.Append(buf, x) shape.
func assignShape(buf []byte, n int) []byte {
	buf = fmt.Append(buf, n) // want `fmt\.Append with a single int/uint/bool/float operand`
	return buf
}

// Untyped constants materialize as their default types: int, rune (int32),
// float64 and bool.
func constInt(buf []byte) []byte {
	return fmt.Append(buf, 42) // want `fmt\.Append with a single int/uint/bool/float operand`
}

func constRune(buf []byte) []byte {
	return fmt.Append(buf, 'a') // want `fmt\.Append with a single int/uint/bool/float operand`
}

func constFloat(buf []byte) []byte {
	return fmt.Append(buf, 2.5) // want `fmt\.Append with a single int/uint/bool/float operand`
}

func constBool(buf []byte) []byte {
	return fmt.Append(buf, true) // want `fmt\.Append with a single int/uint/bool/float operand`
}

// An expression operand stays byte-verbatim inside the widening wrapper.
func exprOperand(buf []byte, n int) []byte {
	return fmt.Append(buf, n*2+1) // want `fmt\.Append with a single int/uint/bool/float operand`
}

// A named []byte DESTINATION is fixable: fmt.Append and strconv.Append* both
// take and return the unnamed []byte, so the named destination type-checks
// identically on both sides (unlike PS5033's builtin-append rewrite, whose
// result takes the destination's own type).
type namedBuf []byte

func namedDest(nb namedBuf, n int) []byte {
	return fmt.Append(nb, n) // want `fmt\.Append with a single int/uint/bool/float operand`
}

// A nil-literal destination is fixable for the same reason.
func nilDest(n int) []byte {
	return fmt.Append(nil, n) // want `fmt\.Append with a single int/uint/bool/float operand`
}

// A comment BETWEEN buf and x survives only outside the rewritten spans —
// this one sits inside the replaced comma span, so the fix is withheld and
// the report stays advisory.
func commentBetweenArgs(buf []byte, n int) []byte {
	return fmt.Append(buf /* count */, n) // want `fmt\.Append with a single int/uint/bool/float operand`
}

// A comment in the closing span (after the operand) is swallowed by the
// suffix splice — fix withheld, advisory report.
func commentAfterOperand(buf []byte, n int) []byte {
	return fmt.Append(buf, n /* count */) // want `fmt\.Append with a single int/uint/bool/float operand`
}

// strconv is shadowed at the call site: the rewrite could not reference the
// package — reported, no fix.
func shadowedStrconv(buf []byte, n int) []byte {
	strconv := n
	_ = strconv
	return fmt.Append(buf, n) // want `fmt\.Append with a single int/uint/bool/float operand`
}

// --- SILENT NEGATIVES ---

// THE CRITICAL GUARD: %v (and therefore fmt.Append) HONORS fmt.Stringer.
// MyInt has a String() method, so fmt prints "MyInt" — NOT the decimal
// digits — and a strconv rewrite would change the output. Named types stay
// SILENT (not even advisory: the message would be wrong for them).
type MyInt int

func (MyInt) String() string { return "MyInt" }

func namedStringer(buf []byte, m MyInt) []byte {
	return fmt.Append(buf, m)
}

// A named integer WITHOUT methods is silent too — the unnamed-only guard
// does not depend on whether a String method exists today.
type port int

func namedPlain(buf []byte, p port) []byte {
	return fmt.Append(buf, p)
}

// The same guard for floats.
type Celsius float64

func (Celsius) String() string { return "warm" }

func namedFloat(buf []byte, c Celsius) []byte {
	return fmt.Append(buf, c)
}

// A single STRING operand is PS5033's (append(buf, s...)) — silent here.
func stringOperand(buf []byte, s string) []byte {
	return fmt.Append(buf, s)
}

// %v over a []byte prints the decimal slice representation "[104 105]" —
// not this pattern at all.
func byteSliceOperand(buf, b []byte) []byte {
	return fmt.Append(buf, b)
}

// A complex operand has no single strconv equivalent of fmt's (re+imi)
// rendering — silent.
func complexOperand(buf []byte, c complex128) []byte {
	return fmt.Append(buf, c)
}

// nil prints "<nil>" — silent.
func nilOperand(buf []byte) []byte {
	return fmt.Append(buf, nil)
}

// Two operands engage Sprint's between-operand spacing rule — silent.
func twoOperands(buf []byte, a, b int) []byte {
	return fmt.Append(buf, a, b)
}

// Zero operands append nothing — silent.
func zeroOperands(buf []byte) []byte {
	return fmt.Append(buf)
}

// A spread passes an unknown number of operands — silent.
func spread(buf []byte, xs []any) []byte {
	return fmt.Append(buf, xs...)
}

// Appendf and Appendln are different functions (PS5015's and out of scope).
func appendf(buf []byte, n int) []byte {
	return fmt.Appendf(buf, "%v", n)
}

func appendln(buf []byte, n int) []byte {
	return fmt.Appendln(buf, n)
}

// An interface operand's dynamic type is unknown — silent.
func anyOperand(buf []byte, v any) []byte {
	return fmt.Append(buf, v)
}
