package ps5015

import (
	"fmt"
	"strconv"
)

// Keeps fmt and strconv referenced so the rewrites below neither orphan the
// fmt import nor need to add strconv (importadd.go and orphan.go exercise
// those paths).
func keep() { fmt.Println(strconv.Itoa(1)) }

// --- POSITIVES: one bare scalar verb, unnamed operand, unnamed []byte dest ---

func decInt(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%d", i) // want `fmt\.Appendf of a single %d integer value`
}

// An int64 operand needs no widening wrapper.
func decInt64(buf []byte, i int64) []byte {
	return fmt.Appendf(buf, "%d", i) // want `fmt\.Appendf of a single %d integer value`
}

func decUint(buf []byte, u uint) []byte {
	return fmt.Appendf(buf, "%d", u) // want `fmt\.Appendf of a single %d integer value`
}

// A uint64 operand needs no widening wrapper.
func decUint64(buf []byte, u uint64) []byte {
	return fmt.Appendf(buf, "%d", u) // want `fmt\.Appendf of a single %d integer value`
}

func decUintptr(buf []byte, p uintptr) []byte {
	return fmt.Appendf(buf, "%d", p) // want `fmt\.Appendf of a single %d integer value`
}

func binInt8(buf []byte, i int8) []byte {
	return fmt.Appendf(buf, "%b", i) // want `fmt\.Appendf of a single %b integer value`
}

func octUint8(buf []byte, u uint8) []byte {
	return fmt.Appendf(buf, "%o", u) // want `fmt\.Appendf of a single %o integer value`
}

func hexInt16(buf []byte, i int16) []byte {
	return fmt.Appendf(buf, "%x", i) // want `fmt\.Appendf of a single %x integer value`
}

// The canonical buf = Appendf(buf, ...) shape, %x over an unsigned width.
func hexAssign(buf []byte, u uint16) []byte {
	buf = fmt.Appendf(buf, "%x", u) // want `fmt\.Appendf of a single %x integer value`
	return buf
}

func boolean(buf []byte, b bool) []byte {
	return fmt.Appendf(buf, "%t", b) // want `fmt\.Appendf of a single %t value`
}

func f64(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%g", f) // want `fmt\.Appendf of a single %g float value`
}

// float32 keeps its own rounding: bitSize 32 with a value-preserving widening.
func f32(buf []byte, f float32) []byte {
	return fmt.Appendf(buf, "%g", f) // want `fmt\.Appendf of a single %g float value`
}

func quoted(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%q", s) // want `fmt\.Appendf\(buf, "%q", s\) on a string`
}

func quotedRune(buf []byte, r rune) []byte {
	return fmt.Appendf(buf, "%q", r) // want `fmt\.Appendf\(buf, "%q", r\) on a rune`
}

// Untyped constants materialize as their default types.
func untypedInt(buf []byte) []byte {
	return fmt.Appendf(buf, "%d", 42) // want `fmt\.Appendf of a single %d integer value`
}

func untypedFloat(buf []byte) []byte {
	return fmt.Appendf(buf, "%g", 2.5) // want `fmt\.Appendf of a single %g float value`
}

func untypedString(buf []byte) []byte {
	return fmt.Appendf(buf, "%q", "hi\xff") // want `fmt\.Appendf\(buf, "%q", s\) on a string`
}

// A side-effecting operand stays verbatim in place, evaluated exactly once.
func sideEffect(buf []byte, next func() int) []byte {
	return fmt.Appendf(buf, "%d", next()) // want `fmt\.Appendf of a single %d integer value`
}

// A raw-string format literal unquotes to the same lone verb.
func rawFormat(buf []byte, i int) []byte {
	return fmt.Appendf(buf, `%d`, i) // want `fmt\.Appendf of a single %d integer value`
}

// --- ADVISORY: reported but NOT fixed (not provably bit-identical) ---

type MyInt int

// A NAMED operand type may implement fmt.Stringer/fmt.Formatter.
func namedOperand(buf []byte, m MyInt) []byte {
	return fmt.Appendf(buf, "%d", m) // want `fmt\.Appendf of a single %d integer value`
}

type MyStr string

func namedString(buf []byte, s MyStr) []byte {
	return fmt.Appendf(buf, "%q", s) // want `fmt\.Appendf\(buf, "%q", s\) on a string`
}

type ByteBuf []byte

// A NAMED []byte destination is not the unnamed []byte PS2141's guard demands.
func namedDest(buf ByteBuf, i int) []byte {
	return fmt.Appendf(buf, "%d", i) // want `fmt\.Appendf of a single %d integer value`
}

// An untyped nil destination is not an unnamed []byte either.
func nilDest(i int) []byte {
	return fmt.Appendf(nil, "%d", i) // want `fmt\.Appendf of a single %d integer value`
}

// A comment inside the rewritten scaffolding withholds the fix.
func commented(buf []byte, i int) []byte {
	return fmt.Appendf(buf /* scaffolding comment */, "%d", i) // want `fmt\.Appendf of a single %d integer value`
}
