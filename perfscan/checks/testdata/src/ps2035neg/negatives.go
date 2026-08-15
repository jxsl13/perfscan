package ps2035neg

import "fmt"

// NEGATIVES: none of these may produce a PS2035 diagnostic at all.

// NAMED types are skipped entirely, not reported: %v HONORS fmt.Stringer
// and fmt.Formatter, so a named type may print via its String() method —
// even an advisory "use strconv" would be wrong output-wise.
type MyInt int

func (m MyInt) String() string { return "custom" }

func namedInt(buf []byte, m MyInt) []byte {
	return fmt.Appendf(buf, "%v", m)
}

type MyBool bool

func namedBool(buf []byte, m MyBool) []byte {
	return fmt.Appendf(buf, "%v", m)
}

type MyFloat float64

func namedFloat(buf []byte, m MyFloat) []byte {
	return fmt.Appendf(buf, "%v", m)
}

// Strings are out of scope: the lone %s belongs to PS2141, and %v of a
// string is the same identity — not this check's scalar shape.
func vString(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%v", s)
}

// %v over a []byte prints the "[104 105]" element list, not the bytes.
func vBytes(buf []byte, bs []byte) []byte {
	return fmt.Appendf(buf, "%v", bs)
}

// Complex kinds have no strconv.Append* twin.
func vComplex(buf []byte, c complex128) []byte {
	return fmt.Appendf(buf, "%v", c)
}

// Interfaces, pointers and structs may hold anything.
func vAny(buf []byte, x any) []byte {
	return fmt.Appendf(buf, "%v", x)
}

func vErr(buf []byte, err error) []byte {
	return fmt.Appendf(buf, "%v", err)
}

func vPtr(buf []byte, p *int) []byte {
	return fmt.Appendf(buf, "%v", p)
}

// An untyped nil operand is not a scalar.
func vNilOperand(buf []byte) []byte {
	return fmt.Appendf(buf, "%v", nil)
}

// Literal text or whitespace around the verb is real formatting.
func withText(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%v items", i)
}

func withPrefix(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "id=%v", i)
}

func withNewline(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%v\n", i)
}

// Flags and width change the output.
func plusFlag(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%+v", i)
}

func hashFlag(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%#v", i)
}

func width(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%2v", i)
}

// Other bare verbs are PS5015's territory (%d/%b/%o/%x/%t/%g/%q) or
// PS2141's (%s) — never double-reported here.
func dVerb(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%d", i)
}

func tVerb(buf []byte, b bool) []byte {
	return fmt.Appendf(buf, "%t", b)
}

func gVerb(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%g", f)
}

func sVerb(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%s", s)
}

// A non-literal format proves nothing.
func varFormat(buf []byte, i int) []byte {
	format := "%v"
	return fmt.Appendf(buf, format, i)
}

// A spread call passes an unknown number of arguments.
func spread(buf []byte, args ...any) []byte {
	return fmt.Appendf(buf, "%v", args...)
}

// Two verbs / two operands are not the single-scalar shape.
func twoVerbs(buf []byte, a, b int) []byte {
	return fmt.Appendf(buf, "%v%v", a, b)
}

// A shadowed fmt is not the fmt package.
type fakeFmt struct{}

func (fakeFmt) Appendf(b []byte, format string, args ...any) []byte { return b }

func shadowedFmt(buf []byte, i int) []byte {
	fmt := fakeFmt{}
	return fmt.Appendf(buf, "%v", i)
}
