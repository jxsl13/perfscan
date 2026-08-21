package ps5040

import (
	"fmt"
	"unicode/utf8"
)

type buffer []byte

func sideEffect() rune { return 'z' }

// Keep the unicode/utf8 import referenced so the fixes reuse the existing
// qualifier and add no import edit (importadd.go covers the add path).
var _ = utf8.UTFMax

// --- fixable: dst is the unnamed []byte, "%c", the operand fits int32 ---

func runeVar(dst []byte, r rune) []byte {
	return fmt.Appendf(dst, "%c", r) // want `fmt\.Appendf\(buf, "%c", r\) parses the format and boxes r to UTF-8-encode one rune; utf8\.AppendRune\(buf, r\) appends the identical bytes directly`
}

func runeVarAgain(dst []byte, r rune) []byte {
	dst = fmt.Appendf(dst, "%c", r) // want `fmt\.Appendf\(buf, "%c", r\) parses the format and boxes r to UTF-8-encode one rune; utf8\.AppendRune\(buf, r\) appends the identical bytes directly`
	return dst
}

// int32 IS rune: the operand is assignable and kept verbatim.
func int32Var(dst []byte, i int32) []byte {
	return fmt.Appendf(dst, "%c", i) // want `fmt\.Appendf\(buf, "%c", r\) parses the format and boxes r to UTF-8-encode one rune; utf8\.AppendRune\(buf, r\) appends the identical bytes directly`
}

// The narrower widths widen value-preservingly as rune(x).
func byteVar(dst []byte, c byte) []byte {
	return fmt.Appendf(dst, "%c", c) // want `fmt\.Appendf\(buf, "%c", r\) parses the format and boxes r to UTF-8-encode one rune; utf8\.AppendRune\(buf, r\) appends the identical bytes directly`
}

func int16Var(dst []byte, s int16) []byte {
	return fmt.Appendf(dst, "%c", s) // want `fmt\.Appendf\(buf, "%c", r\) parses the format and boxes r to UTF-8-encode one rune; utf8\.AppendRune\(buf, r\) appends the identical bytes directly`
}

// A side-effecting operand is evaluated once, in place.
func sideEffectOperand(dst []byte) []byte {
	return fmt.Appendf(dst, "%c", sideEffect()) // want `fmt\.Appendf\(buf, "%c", r\) parses the format and boxes r to UTF-8-encode one rune; utf8\.AppendRune\(buf, r\) appends the identical bytes directly`
}

// --- advisory: reported, no fix ---

// A NAMED byte-slice dst: utf8.AppendRune returns []byte, so fixing would
// change the expression's static type.
func namedDst(dst buffer, r rune) buffer {
	return fmt.Appendf(dst, "%c", r) // want `fmt\.Appendf\(buf, "%c", r\) parses the format and boxes r to UTF-8-encode one rune; utf8\.AppendRune\(buf, r\) appends the identical bytes directly`
}

// A comment inside the rewritten scaffolding would be destroyed — advisory.
func commentInside(dst []byte, r rune) []byte {
	return fmt.Appendf(dst /* keep */, "%c", r) // want `fmt\.Appendf\(buf, "%c", r\) parses the format and boxes r to UTF-8-encode one rune; utf8\.AppendRune\(buf, r\) appends the identical bytes directly`
}

// --- negatives: silent ---

// A wider integer type: a value above MaxInt32 makes fmt emit U+FFFD while
// rune(x) would truncate to a different code point.
func wideInt(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%c", n)
}

func uint32Var(dst []byte, u uint32) []byte {
	return fmt.Appendf(dst, "%c", u)
}

// A constant rune operand: rune(const) could overflow int32 at compile time.
func constRune(dst []byte) []byte {
	return fmt.Appendf(dst, "%c", 'A')
}

// Not exactly "%c".
func widthFlag(dst []byte, r rune) []byte {
	return fmt.Appendf(dst, "%2c", r)
}

func otherVerb(dst []byte, r rune) []byte {
	return fmt.Appendf(dst, "%d", r)
}
