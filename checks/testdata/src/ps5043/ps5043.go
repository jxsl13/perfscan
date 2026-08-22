package ps5043

import (
	"fmt"
	"strconv"
)

type buffer []byte

type myInt int

func (m myInt) Format(f fmt.State, verb rune) { fmt.Fprint(f, "X") }

func sideEffect() int { return 7 }

var _ = strconv.Quote

// --- fixable: unnamed []byte dst, "%x", integer operand ---

func intVar(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%x", n) // want `fmt\.Appendf\(buf, "%x", n\) parses the format and boxes n to hex-print one integer; strconv\.AppendInt/AppendUint appends the lowercase hex digits directly`
}

func int64Var(dst []byte, n int64) []byte {
	return fmt.Appendf(dst, "%x", n) // want `fmt\.Appendf\(buf, "%x", n\) parses the format and boxes n to hex-print one integer; strconv\.AppendInt/AppendUint appends the lowercase hex digits directly`
}

func uintVar(dst []byte, u uint) []byte {
	return fmt.Appendf(dst, "%x", u) // want `fmt\.Appendf\(buf, "%x", n\) parses the format and boxes n to hex-print one integer; strconv\.AppendInt/AppendUint appends the lowercase hex digits directly`
}

func uint64Var(dst []byte, u uint64) []byte {
	return fmt.Appendf(dst, "%x", u) // want `fmt\.Appendf\(buf, "%x", n\) parses the format and boxes n to hex-print one integer; strconv\.AppendInt/AppendUint appends the lowercase hex digits directly`
}

func byteVar(dst []byte, c byte) []byte {
	return fmt.Appendf(dst, "%x", c) // want `fmt\.Appendf\(buf, "%x", n\) parses the format and boxes n to hex-print one integer; strconv\.AppendInt/AppendUint appends the lowercase hex digits directly`
}

func sideEffectOperand(dst []byte) []byte {
	return fmt.Appendf(dst, "%x", sideEffect()) // want `fmt\.Appendf\(buf, "%x", n\) parses the format and boxes n to hex-print one integer; strconv\.AppendInt/AppendUint appends the lowercase hex digits directly`
}

// --- advisory: reported, no fix ---

func namedDst(dst buffer, n int) buffer {
	return fmt.Appendf(dst, "%x", n) // want `fmt\.Appendf\(buf, "%x", n\) parses the format and boxes n to hex-print one integer; strconv\.AppendInt/AppendUint appends the lowercase hex digits directly`
}

func commentInside(dst []byte, n int) []byte {
	return fmt.Appendf(dst /* keep */, "%x", n) // want `fmt\.Appendf\(buf, "%x", n\) parses the format and boxes n to hex-print one integer; strconv\.AppendInt/AppendUint appends the lowercase hex digits directly`
}

// --- negatives: silent ---

// A named integer type with a Format method: "%x" formats via Format().
func namedInt(dst []byte, m myInt) []byte {
	return fmt.Appendf(dst, "%x", m)
}

func widthFlag(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%3x", n)
}

// The uppercase "%X" produces upper-case digits AppendInt cannot — silent.
func upperHex(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%X", n)
}

func otherVerb(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%o", n)
}

// "%x" of a float is a hex-float form, not the integer shape.
func floatOperand(dst []byte, x float64) []byte {
	return fmt.Appendf(dst, "%x", x)
}
