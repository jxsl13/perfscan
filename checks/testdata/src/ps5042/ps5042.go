package ps5042

import (
	"fmt"
	"strconv"
)

type buffer []byte

type myInt int

func (m myInt) Format(f fmt.State, verb rune) { fmt.Fprint(f, "X") }

func sideEffect() int { return 7 }

var _ = strconv.Quote

// --- fixable: unnamed []byte dst, "%d", integer operand ---

func intVar(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%d", n) // want `fmt\.Appendf\(buf, "%d", n\) parses the format and boxes n to print one integer; strconv\.AppendInt/AppendUint appends the decimal digits directly`
}

func int64Var(dst []byte, n int64) []byte {
	return fmt.Appendf(dst, "%d", n) // want `fmt\.Appendf\(buf, "%d", n\) parses the format and boxes n to print one integer; strconv\.AppendInt/AppendUint appends the decimal digits directly`
}

func uintVar(dst []byte, u uint) []byte {
	return fmt.Appendf(dst, "%d", u) // want `fmt\.Appendf\(buf, "%d", n\) parses the format and boxes n to print one integer; strconv\.AppendInt/AppendUint appends the decimal digits directly`
}

func uint64Var(dst []byte, u uint64) []byte {
	return fmt.Appendf(dst, "%d", u) // want `fmt\.Appendf\(buf, "%d", n\) parses the format and boxes n to print one integer; strconv\.AppendInt/AppendUint appends the decimal digits directly`
}

func byteVar(dst []byte, c byte) []byte {
	return fmt.Appendf(dst, "%d", c) // want `fmt\.Appendf\(buf, "%d", n\) parses the format and boxes n to print one integer; strconv\.AppendInt/AppendUint appends the decimal digits directly`
}

func sideEffectOperand(dst []byte) []byte {
	return fmt.Appendf(dst, "%d", sideEffect()) // want `fmt\.Appendf\(buf, "%d", n\) parses the format and boxes n to print one integer; strconv\.AppendInt/AppendUint appends the decimal digits directly`
}

// --- advisory: reported, no fix ---

func namedDst(dst buffer, n int) buffer {
	return fmt.Appendf(dst, "%d", n) // want `fmt\.Appendf\(buf, "%d", n\) parses the format and boxes n to print one integer; strconv\.AppendInt/AppendUint appends the decimal digits directly`
}

func commentInside(dst []byte, n int) []byte {
	return fmt.Appendf(dst /* keep */, "%d", n) // want `fmt\.Appendf\(buf, "%d", n\) parses the format and boxes n to print one integer; strconv\.AppendInt/AppendUint appends the decimal digits directly`
}

// --- negatives: silent ---

// A named integer type with a Format method: "%d" formats via Format().
func namedInt(dst []byte, m myInt) []byte {
	return fmt.Appendf(dst, "%d", m)
}

func widthFlag(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%3d", n)
}

func otherVerb(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%x", n)
}

func floatOperand(dst []byte, x float64) []byte {
	return fmt.Appendf(dst, "%d", x)
}
