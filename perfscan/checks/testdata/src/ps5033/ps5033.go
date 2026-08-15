package ps5033

import (
	"fmt"
	"strings"
)

// Keeps fmt referenced so the rewrites below do not orphan the import (orphan.go
// exercises the orphan path).
func keep() { fmt.Println("x") }

// --- POSITIVES: a single plain-string operand, unnamed []byte dest ---

func str(buf []byte, s string) []byte {
	return fmt.Append(buf, s) // want `fmt\.Append with a single string operand`
}

// The operand is a call — kept verbatim.
func callArg(buf []byte, s string) []byte {
	return fmt.Append(buf, strings.ToUpper(s)) // want `fmt\.Append with a single string operand`
}

// An untyped string constant operand defaults to string.
func lit(buf []byte) []byte {
	return fmt.Append(buf, "literal") // want `fmt\.Append with a single string operand`
}

// A conversion pins the static type to the predeclared string.
func conv(buf []byte, b []byte) []byte {
	return fmt.Append(buf, string(b)) // want `fmt\.Append with a single string operand`
}

// A trailing comma in the scaffolding span is absorbed by the rewrite.
func trailingComma(buf []byte, s string) []byte {
	return fmt.Append( // want `fmt\.Append with a single string operand`
		buf,
		s,
	)
}

// A comment between buf and s is outside every rewritten span and survives.
func commentBetween(buf []byte, s string) []byte {
	return fmt.Append(buf /* kept */, s) // want `fmt\.Append with a single string operand`
}

// --- ADVISORY: reported but NOT fixed (not provably bit-identical) ---

type MyStr string

// A NAMED string type may implement fmt.Stringer/fmt.Formatter, which %v
// honors and append does not.
func named(buf []byte, s MyStr) []byte {
	return fmt.Append(buf, s) // want `fmt\.Append with a single string operand`
}

type ByteBuf []byte

// A NAMED []byte destination changes the returned slice type.
func namedDest(buf ByteBuf, s string) ByteBuf {
	return fmt.Append(buf, s) // want `fmt\.Append with a single string operand`
}

// A comment inside the rewritten trailing-scaffolding span would be destroyed.
func commentInScaffolding(buf []byte, s string) []byte {
	return fmt.Append(buf, s /* keep me */) // want `fmt\.Append with a single string operand`
}

// --- NEGATIVES: not reported ---

// Two or more operands engage Sprint's between-operand spacing rule.
func twoOps(buf []byte, s, t string) []byte {
	return fmt.Append(buf, s, t)
}

// No operands appends nothing — not this pattern.
func noOps(buf []byte) []byte {
	return fmt.Append(buf)
}

// %v over a []byte prints the decimal slice representation "[104 105]" —
// nothing like append.
func byteSliceOp(buf []byte, b []byte) []byte {
	return fmt.Append(buf, b)
}

// %v over an int is different formatting, not a string append.
func intOp(buf []byte, n int) []byte {
	return fmt.Append(buf, n)
}

// An interface operand's dynamic type is unknown.
func anyOp(buf []byte, v any) []byte {
	return fmt.Append(buf, v)
}

// An error operand prints its Error() result.
func errOp(buf []byte, err error) []byte {
	return fmt.Append(buf, err)
}

// nil prints "<nil>".
func nilOp(buf []byte) []byte {
	return fmt.Append(buf, nil)
}

// A spread call passes an unknown number of operands.
func spread(buf []byte, args ...any) []byte {
	return fmt.Append(buf, args...)
}

// Appendln adds a newline; Appendf is PS2141's.
func ln(buf []byte, s string) []byte {
	return fmt.Appendln(buf, s)
}

// A shadowed fmt is not the fmt package.
type fakeFmt struct{}

func (fakeFmt) Append(b []byte, args ...any) []byte { return b }

func shadowed(buf []byte, s string) []byte {
	fmt := fakeFmt{}
	return fmt.Append(buf, s)
}
