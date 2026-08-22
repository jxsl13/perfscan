package ps5041

import (
	"fmt"
	"strconv"
)

type buffer []byte

type myStr string

func (m myStr) String() string { return "S" }

func sideEffect() string { return "z" }

// Keep the strconv import referenced so the fixes reuse the existing
// qualifier and add no import edit (ps5041add covers the add path).
var _ = strconv.Quote

// --- fixable: dst is the unnamed []byte, "%q", operand a plain string ---

func strVar(dst []byte, s string) []byte {
	return fmt.Appendf(dst, "%q", s) // want `fmt\.Appendf\(buf, "%q", s\) parses the format and boxes s to quote one string; strconv\.AppendQuote\(buf, s\) appends the identical Go string literal directly`
}

func strVarAgain(dst []byte, s string) []byte {
	dst = fmt.Appendf(dst, "%q", s) // want `fmt\.Appendf\(buf, "%q", s\) parses the format and boxes s to quote one string; strconv\.AppendQuote\(buf, s\) appends the identical Go string literal directly`
	return dst
}

// A constant string operand: strconv.AppendQuote quotes it the same way.
func constStr(dst []byte) []byte {
	return fmt.Appendf(dst, "%q", "lit\tx") // want `fmt\.Appendf\(buf, "%q", s\) parses the format and boxes s to quote one string; strconv\.AppendQuote\(buf, s\) appends the identical Go string literal directly`
}

// A side-effecting operand is evaluated once, in place.
func sideEffectOperand(dst []byte) []byte {
	return fmt.Appendf(dst, "%q", sideEffect()) // want `fmt\.Appendf\(buf, "%q", s\) parses the format and boxes s to quote one string; strconv\.AppendQuote\(buf, s\) appends the identical Go string literal directly`
}

// --- advisory: reported, no fix ---

// A NAMED byte-slice dst: AppendQuote returns []byte, so fixing would change
// the expression's static type.
func namedDst(dst buffer, s string) buffer {
	return fmt.Appendf(dst, "%q", s) // want `fmt\.Appendf\(buf, "%q", s\) parses the format and boxes s to quote one string; strconv\.AppendQuote\(buf, s\) appends the identical Go string literal directly`
}

// A comment inside the rewritten scaffolding would be destroyed — advisory.
func commentInside(dst []byte, s string) []byte {
	return fmt.Appendf(dst /* keep */, "%q", s) // want `fmt\.Appendf\(buf, "%q", s\) parses the format and boxes s to quote one string; strconv\.AppendQuote\(buf, s\) appends the identical Go string literal directly`
}

// --- negatives: silent ---

// A NAMED string type with a String method: "%q" quotes String(), not the raw
// value, so the rewrite would diverge.
func namedStr(dst []byte, m myStr) []byte {
	return fmt.Appendf(dst, "%q", m)
}

// The ASCII form is a different verb.
func asciiForm(dst []byte, s string) []byte {
	return fmt.Appendf(dst, "%+q", s)
}

// A non-string operand.
func intOperand(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%q", n)
}
