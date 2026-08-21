package ps2041

import (
	"fmt"
	"os"
	"strings"
)

// Keeps fmt referenced so the rewrites below do not orphan the import (orphan.go
// exercises the orphan path).
func keep() { fmt.Println("x") }

// --- POSITIVES: string operands, unnamed []byte dest ---

func one(buf []byte, s string) []byte {
	return fmt.Appendln(buf, s) // want `fmt\.Appendln over string operands`
}

func two(buf []byte, a, b string) []byte {
	return fmt.Appendln(buf, a, b) // want `fmt\.Appendln over string operands`
}

func three(buf []byte, a, b, c string) []byte {
	return fmt.Appendln(buf, a, b, c) // want `fmt\.Appendln over string operands`
}

// The FIRST operand may be any string expression — both forms evaluate buf,
// then it, before the chain's first write.
func firstCall(buf []byte, a, b string) []byte {
	return fmt.Appendln(buf, strings.ToUpper(a), b) // want `fmt\.Appendln over string operands`
}

// Untyped string constants default to string; a package-qualified identifier
// is inert.
func lits(buf []byte, a string) []byte {
	return fmt.Appendln(buf, a, "-", os.DevNull) // want `fmt\.Appendln over string operands`
}

// A comment between buf and the first operand is outside every rewritten
// span and survives.
func commentAfterBuf(buf []byte, s string) []byte {
	return fmt.Appendln(buf /* kept */, s) // want `fmt\.Appendln over string operands`
}

// A trailing comma in the final scaffolding span is absorbed by the rewrite.
func trailingComma(buf []byte, s string) []byte {
	return fmt.Appendln( // want `fmt\.Appendln over string operands`
		buf,
		s,
	)
}

// --- ADVISORY: reported but NOT fixed (not provably bit-identical) ---

type MyStr string

// A NAMED string type may implement fmt.Stringer/fmt.Formatter, which %v
// honors and append would not.
func named(buf []byte, a string, s MyStr) []byte {
	return fmt.Appendln(buf, a, s) // want `fmt\.Appendln over string operands`
}

type ByteBuf []byte

// A NAMED []byte destination changes the returned slice type.
func namedDest(buf ByteBuf, a, b string) ByteBuf {
	return fmt.Appendln(buf, a, b) // want `fmt\.Appendln over string operands`
}

// A LATER operand that runs user code is evaluated after the chain's first
// write to buf but before fmt.Appendln's only write — an observable
// difference, so the call keeps the advisory report.
func laterCall(buf []byte, a, b string) []byte {
	return fmt.Appendln(buf, a, strings.ToUpper(b)) // want `fmt\.Appendln over string operands`
}

// An index expression may panic — panic timing would move across the first
// write, so it is not inert either.
func laterIndex(buf []byte, a string, parts []string) []byte {
	return fmt.Appendln(buf, a, parts[0]) // want `fmt\.Appendln over string operands`
}

// A comment between two operands sits inside a rewritten chain link — advisory.
func commentBetweenOperands(buf []byte, a, b string) []byte {
	return fmt.Appendln(buf, a /* keep me */, b) // want `fmt\.Appendln over string operands`
}

// A comment inside the rewritten trailing-scaffolding span would be destroyed.
func commentInScaffolding(buf []byte, s string) []byte {
	return fmt.Appendln(buf, s /* keep me */) // want `fmt\.Appendln over string operands`
}

// --- NEGATIVES: not reported ---

// Zero operands appends just "\n" — out of scope.
func zeroOperands(buf []byte) []byte {
	return fmt.Appendln(buf)
}

// %v over an int is different formatting, not a string append.
func intOp(buf []byte, a string, n int) []byte {
	return fmt.Appendln(buf, a, n)
}

// %v over a []byte prints the decimal slice representation "[104 105]" —
// nothing like appending its bytes.
func byteSliceOp(buf []byte, bs []byte) []byte {
	return fmt.Appendln(buf, bs)
}

// An interface operand's dynamic type is unknown.
func anyOp(buf []byte, v any) []byte {
	return fmt.Appendln(buf, v)
}

// An error operand prints its Error() result.
func errOp(buf []byte, err error) []byte {
	return fmt.Appendln(buf, err)
}

// nil prints "<nil>".
func nilOp(buf []byte) []byte {
	return fmt.Appendln(buf, nil)
}

// A spread call passes an unknown number of operands.
func spread(buf []byte, args ...any) []byte {
	return fmt.Appendln(buf, args...)
}

// fmt.Append has the CONDITIONAL spacing rule — PS5033's territory.
func plainAppend(buf []byte, s string) []byte {
	return fmt.Append(buf, s)
}

// fmt.Sprintln returns a string — PS5029's territory.
func sprintln(a, b string) string {
	return fmt.Sprintln(a, b)
}

// A shadowed fmt is not the fmt package.
type fakeFmt struct{}

func (fakeFmt) Appendln(b []byte, args ...any) []byte { return b }

func shadowed(buf []byte, s string) []byte {
	fmt := fakeFmt{}
	return fmt.Appendln(buf, s)
}
