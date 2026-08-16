package ps2046

import (
	"fmt"
)

// --- REPORTED: lone lowercase %x over a byte slice ---

func plain(buf []byte, bs []byte) []byte {
	return fmt.Appendf(buf, "%x", bs) // want `fmt\.Appendf with a lone %x over a \[\]byte`
}

// The value is a call — still the same shape.
func mk() []byte { return []byte{1, 2} }

func callArg(buf []byte) []byte {
	return fmt.Appendf(buf, "%x", mk()) // want `fmt\.Appendf with a lone %x over a \[\]byte`
}

// A []uint8 spelling is the identical type.
func uint8Slice(buf []byte, bs []uint8) []byte {
	return fmt.Appendf(buf, "%x", bs) // want `fmt\.Appendf with a lone %x over a \[\]byte`
}

type Blob []byte

// A NAMED []byte operand is still reported — the Doc tells the human to
// check for a Format method before rewriting.
func namedOperand(buf []byte, b Blob) []byte {
	return fmt.Appendf(buf, "%x", b) // want `fmt\.Appendf with a lone %x over a \[\]byte`
}

// A NAMED []byte destination is still reported (the []byte result assigns
// back to the named type); advisory, human judges the type change.
func namedDest(buf Blob, bs []byte) Blob {
	return fmt.Appendf(buf, "%x", bs) // want `fmt\.Appendf with a lone %x over a \[\]byte`
}

// A byte-array conversion at the argument is still an unnamed []byte.
func converted(buf []byte, b Blob) []byte {
	return fmt.Appendf(buf, "%x", []byte(b)) // want `fmt\.Appendf with a lone %x over a \[\]byte`
}

// --- NOT REPORTED ---

// %X is uppercase hex digits — hex.AppendEncode is lowercase only.
func upperX(buf []byte, bs []byte) []byte {
	return fmt.Appendf(buf, "%X", bs)
}

// A flagged or widened verb is not the bare hex dump.
func flagged(buf []byte, bs []byte) []byte {
	buf = fmt.Appendf(buf, "%#x", bs)
	buf = fmt.Appendf(buf, "% x", bs)
	return fmt.Appendf(buf, "%8x", bs)
}

// Literal text around the verb disqualifies it.
func withText(buf []byte, bs []byte) []byte {
	buf = fmt.Appendf(buf, "x: %x", bs)
	return fmt.Appendf(buf, "%x\n", bs)
}

// %x over an integer is PS5015's subject, not this check's.
func intVal(buf []byte, n int) []byte {
	return fmt.Appendf(buf, "%x", n)
}

// %x over a string is different plumbing (hex of the string's bytes, but
// hex.AppendEncode takes a []byte) — out of scope.
func strVal(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%x", s)
}

// %x over a float is hexadecimal floating point — different formatting.
func floatVal(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%x", f)
}

type myByte byte

// A named element type makes the slice unassignable to hex's []byte.
func namedElem(buf []byte, bs []myByte) []byte {
	return fmt.Appendf(buf, "%x", bs)
}

// A non-byte slice hex-dumps per element with separators — different output.
func intSlice(buf []byte, xs []int) []byte {
	return fmt.Appendf(buf, "%x", xs)
}

// A spread call is not the three-argument shape.
func spread(buf []byte, args []any) []byte {
	return fmt.Appendf(buf, "%x", args...)
}

// A non-literal format is never matched.
func nonLiteral(buf []byte, format string, bs []byte) []byte {
	return fmt.Appendf(buf, format, bs)
}

// A shadowed fmt is not the fmt package.
type fakeFmt struct{}

func (fakeFmt) Appendf(b []byte, format string, a ...any) []byte { return b }

func shadowed(buf []byte, bs []byte) []byte {
	fmt := fakeFmt{}
	return fmt.Appendf(buf, "%x", bs)
}

// Two operands is not the single-verb shape.
func twoOps(buf []byte, a, b []byte) []byte {
	return fmt.Appendf(buf, "%x", a, b)
}
