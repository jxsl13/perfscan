package ps2040

import (
	"fmt"
	"os"
	"strings"
)

// Keeps fmt referenced so the rewrites below do not orphan the import (orphan.go
// exercises the orphan path).
func keep() { fmt.Println("x") }

// --- POSITIVES: two or more plain-string operands, unnamed []byte dest ---

func two(buf []byte, a, b string) []byte {
	return fmt.Append(buf, a, b) // want `fmt\.Append over two or more string operands`
}

func three(buf []byte, host, port string) []byte {
	return fmt.Append(buf, host, ":", port) // want `fmt\.Append over two or more string operands`
}

// The FIRST operand may be any string expression — both forms evaluate buf,
// then it, before the chain's first write.
func firstCall(buf []byte, a, b string) []byte {
	return fmt.Append(buf, strings.ToUpper(a), b) // want `fmt\.Append over two or more string operands`
}

// A conversion as the first operand pins the static type to the predeclared
// string.
func firstConv(buf []byte, bs []byte, b string) []byte {
	return fmt.Append(buf, string(bs), b) // want `fmt\.Append over two or more string operands`
}

// Untyped string constants default to string; a package-qualified identifier
// is inert.
func lits(buf []byte, a string) []byte {
	return fmt.Append(buf, a, "-", os.DevNull) // want `fmt\.Append over two or more string operands`
}

// A trailing comma in the scaffolding span is absorbed by the rewrite.
func trailingComma(buf []byte, a, b string) []byte {
	return fmt.Append( // want `fmt\.Append over two or more string operands`
		buf,
		a,
		b,
	)
}

// A comment between buf and the first operand is outside every rewritten span
// and survives.
func commentBetween(buf []byte, a, b string) []byte {
	return fmt.Append(buf /* kept */, a, b) // want `fmt\.Append over two or more string operands`
}

// --- ADVISORY: reported but NOT fixed (not provably bit-identical) ---

type MyStr string

// A NAMED string type may implement fmt.Stringer/fmt.Formatter, which %v
// honors and append does not.
func named(buf []byte, a string, s MyStr) []byte {
	return fmt.Append(buf, a, s) // want `fmt\.Append over two or more string operands`
}

type ByteBuf []byte

// A NAMED []byte destination changes the returned slice type.
func namedDest(buf ByteBuf, a, b string) ByteBuf {
	return fmt.Append(buf, a, b) // want `fmt\.Append over two or more string operands`
}

// A LATER operand that runs user code is evaluated after the chain's first
// write to buf but before fmt.Append's only write — an observable difference,
// so the call keeps the advisory report.
func laterCall(buf []byte, a, b string) []byte {
	return fmt.Append(buf, a, strings.ToUpper(b)) // want `fmt\.Append over two or more string operands`
}

// An index expression may panic — panic timing would move across the first
// write, so it is not inert either.
func laterIndex(buf []byte, a string, parts []string) []byte {
	return fmt.Append(buf, a, parts[0]) // want `fmt\.Append over two or more string operands`
}

// A comment inside a rewritten inter-operand span would be destroyed —
// advisory.
func commentedComma(buf []byte, a, b string) []byte {
	return fmt.Append(buf, a /* keep me */, b) // want `fmt\.Append over two or more string operands`
}

// A comment inside the rewritten trailing-scaffolding span would be destroyed
// too.
func commentedTail(buf []byte, a, b string) []byte {
	return fmt.Append(buf, a, b /* keep me */) // want `fmt\.Append over two or more string operands`
}

// --- NEGATIVES: not reported ---

// The single-operand call is PS5033's shape, not PS2040's.
func single(buf []byte, a string) []byte {
	return fmt.Append(buf, a)
}

// No operands appends nothing — not this pattern.
func noOps(buf []byte) []byte {
	return fmt.Append(buf)
}

// %v over a []byte prints the decimal slice representation "[104 105]" —
// nothing like append, so byte-slice operands are a different pattern
// entirely.
func byteSliceOp(buf []byte, a string, bs []byte) []byte {
	return fmt.Append(buf, a, bs)
}

// A non-string operand re-engages Sprint's spacing rule against its neighbors
// and formats differently anyway.
func intOp(buf []byte, a string, n int) []byte {
	return fmt.Append(buf, a, n)
}

// An interface operand's dynamic type is unknown.
func anyOp(buf []byte, a string, v any) []byte {
	return fmt.Append(buf, a, v)
}

// An error operand prints its Error() result.
func errOp(buf []byte, a string, err error) []byte {
	return fmt.Append(buf, a, err)
}

// nil prints "<nil>".
func nilOp(buf []byte, a string) []byte {
	return fmt.Append(buf, a, nil)
}

// A spread call passes an unknown number of operands.
func spread(buf []byte, args ...any) []byte {
	return fmt.Append(buf, args...)
}

// Appendln adds separators and a newline; Appendf is PS2033/PS2141 territory.
func ln(buf []byte, a, b string) []byte {
	return fmt.Appendln(buf, a, b)
}

// A shadowed fmt is not the fmt package.
type fakeFmt struct{}

func (fakeFmt) Append(b []byte, args ...any) []byte { return b }

func shadowed(buf []byte, a, b string) []byte {
	fmt := fakeFmt{}
	return fmt.Append(buf, a, b)
}
