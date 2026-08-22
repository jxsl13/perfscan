package ps2035

import (
	"fmt"
	"strconv"
)

// Keeps fmt and strconv referenced so the rewrites below neither orphan the
// fmt import nor need to add strconv (importadd.go and orphan.go exercise
// those paths).
func keep() { fmt.Println(strconv.Itoa(1)) }

// --- POSITIVES: "%v" over an unnamed scalar, unnamed []byte destination ---

func vInt(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%v", i) // want `fmt\.Appendf of a single %v integer value`
}

// An int64 operand needs no widening wrapper.
func vInt64(buf []byte, i int64) []byte {
	return fmt.Appendf(buf, "%v", i) // want `fmt\.Appendf of a single %v integer value`
}

func vInt8(buf []byte, i int8) []byte {
	return fmt.Appendf(buf, "%v", i) // want `fmt\.Appendf of a single %v integer value`
}

func vInt16(buf []byte, i int16) []byte {
	return fmt.Appendf(buf, "%v", i) // want `fmt\.Appendf of a single %v integer value`
}

func vInt32(buf []byte, i int32) []byte {
	return fmt.Appendf(buf, "%v", i) // want `fmt\.Appendf of a single %v integer value`
}

func vUint(buf []byte, u uint) []byte {
	return fmt.Appendf(buf, "%v", u) // want `fmt\.Appendf of a single %v integer value`
}

// A uint64 operand needs no widening wrapper.
func vUint64(buf []byte, u uint64) []byte {
	return fmt.Appendf(buf, "%v", u) // want `fmt\.Appendf of a single %v integer value`
}

func vUint8(buf []byte, u uint8) []byte {
	return fmt.Appendf(buf, "%v", u) // want `fmt\.Appendf of a single %v integer value`
}

func vUintptr(buf []byte, p uintptr) []byte {
	return fmt.Appendf(buf, "%v", p) // want `fmt\.Appendf of a single %v integer value`
}

// The canonical buf = Appendf(buf, ...) shape.
func vAssign(buf []byte, u uint16) []byte {
	buf = fmt.Appendf(buf, "%v", u) // want `fmt\.Appendf of a single %v integer value`
	return buf
}

func vBool(buf []byte, b bool) []byte {
	return fmt.Appendf(buf, "%v", b) // want `fmt\.Appendf of a single %v bool value`
}

func vF64(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%v", f) // want `fmt\.Appendf of a single %v float value`
}

// float32 keeps its own rounding: bitSize 32 with a value-preserving widening.
func vF32(buf []byte, f float32) []byte {
	return fmt.Appendf(buf, "%v", f) // want `fmt\.Appendf of a single %v float value`
}

// Untyped constants materialize as their default types.
func untypedInt(buf []byte) []byte {
	return fmt.Appendf(buf, "%v", 42) // want `fmt\.Appendf of a single %v integer value`
}

func untypedFloat(buf []byte) []byte {
	return fmt.Appendf(buf, "%v", 2.5) // want `fmt\.Appendf of a single %v float value`
}

func untypedBool(buf []byte) []byte {
	return fmt.Appendf(buf, "%v", true) // want `fmt\.Appendf of a single %v bool value`
}

// A side-effecting operand stays verbatim in place, evaluated exactly once.
func sideEffect(buf []byte, next func() int) []byte {
	return fmt.Appendf(buf, "%v", next()) // want `fmt\.Appendf of a single %v integer value`
}

// A raw-string format literal unquotes to the same lone verb.
func rawFormat(buf []byte, i int) []byte {
	return fmt.Appendf(buf, `%v`, i) // want `fmt\.Appendf of a single %v integer value`
}

// --- ADVISORY: reported but NOT fixed (destination/scaffolding guards) ---

type ByteBuf []byte

// A NAMED []byte destination is not the unnamed []byte PS2141's guard demands.
func namedDest(buf ByteBuf, i int) []byte {
	return fmt.Appendf(buf, "%v", i) // want `fmt\.Appendf of a single %v integer value`
}

// An untyped nil destination is not an unnamed []byte either.
func nilDest(i int) []byte {
	return fmt.Appendf(nil, "%v", i) // want `fmt\.Appendf of a single %v integer value`
}

// A comment inside the rewritten scaffolding withholds the fix.
func commented(buf []byte, i int) []byte {
	return fmt.Appendf(buf /* scaffolding comment */, "%v", i) // want `fmt\.Appendf of a single %v integer value`
}
