package ps2141

import (
	"fmt"
	"strings"
)

// Keeps fmt referenced so the rewrites below do not orphan the import (orphan.go
// exercises the orphan path).
func keep() { fmt.Println("x") }

// --- POSITIVES: lone %s over a string or unnamed []byte, unnamed []byte dest ---

func str(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%s", s) // want `fmt\.Appendf with a lone %s over a string/\[\]byte`
}

func bs(buf []byte, b []byte) []byte {
	return fmt.Appendf(buf, "%s", b) // want `fmt\.Appendf with a lone %s over a string/\[\]byte`
}

// The value is a call — kept verbatim.
func callArg(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%s", strings.ToUpper(s)) // want `fmt\.Appendf with a lone %s over a string/\[\]byte`
}

// An untyped string constant argument defaults to string.
func lit(buf []byte) []byte {
	return fmt.Appendf(buf, "%s", "literal") // want `fmt\.Appendf with a lone %s over a string/\[\]byte`
}

// --- ADVISORY: reported but NOT fixed (not provably bit-identical) ---

type MyStr string

// A NAMED string type may implement fmt.Stringer/fmt.Formatter.
func named(buf []byte, s MyStr) []byte {
	return fmt.Appendf(buf, "%s", s) // want `fmt\.Appendf with a lone %s over a string/\[\]byte`
}

type ByteBuf []byte

// A NAMED []byte destination changes the returned slice type.
func namedDest(buf ByteBuf, s string) ByteBuf {
	return fmt.Appendf(buf, "%s", s) // want `fmt\.Appendf with a lone %s over a string/\[\]byte`
}

// --- NEGATIVES: not reported ---

// Not a lone %s.
func withText(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "got %s", s)
}

func withNewline(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%s\n", s)
}

// %s over an int is different formatting, not a string append.
func intVal(buf []byte, n int) []byte {
	return fmt.Appendf(buf, "%s", n)
}

// A spread call passes an unknown number of arguments.
func spread(buf []byte, args ...any) []byte {
	return fmt.Appendf(buf, "%s", args...)
}

// A non-literal format proves nothing.
func varFormat(buf []byte, s string) []byte {
	format := "%s"
	return fmt.Appendf(buf, format, s)
}

// A shadowed fmt is not the fmt package.
type fakeFmt struct{}

func (fakeFmt) Appendf(b []byte, format string, args ...any) []byte { return b }

func shadowed(buf []byte, s string) []byte {
	fmt := fakeFmt{}
	return fmt.Appendf(buf, "%s", s)
}
