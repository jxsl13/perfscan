package ps5015neg

import "fmt"

// NEGATIVES: none of these may produce a PS5015 diagnostic at all.

// Literal text or whitespace around the verb is real formatting.
func withText(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%d items", i)
}

func withPrefix(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "id=%d", i)
}

func withNewline(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%d\n", i)
}

// Flags, width and precision change the output.
func plusFlag(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%+d", i)
}

func width(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%2d", i)
}

func altOctal(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%#o", i)
}

func zeroPad(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%08b", i)
}

// %X prints UPPERCASE hex digits — not FormatInt's lowercase.
func upperHex(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%X", i)
}

// %e/%f default to 6 digits, which AppendFloat(-1) does not reproduce.
func expVerb(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%e", f)
}

func fixedVerb(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%f", f)
}

// %v and %s are other checks' patterns (PS2141 owns the lone %s).
func vVerb(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%v", i)
}

func sVerb(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%s", s)
}

// A literal percent sign is not a verb.
func escapedPercent(buf []byte, i int) []byte {
	_ = i
	return fmt.Appendf(buf, "%%d")
}

// %x over a []byte or string hex-dumps the BYTES; over a float it is
// hexadecimal floating point — neither is the integer arm.
func hexBytes(buf []byte, bs []byte) []byte {
	return fmt.Appendf(buf, "%x", bs)
}

func hexString(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%x", s)
}

func hexFloat(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%x", f)
}

// %b over a float is its binary-exponent form, not integer base 2.
func binFloat(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%b", f)
}

// %q over a []byte or a non-rune integer is out of scope.
func quoteBytes(buf []byte, bs []byte) []byte {
	return fmt.Appendf(buf, "%q", bs)
}

func quoteInt(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%q", i)
}

func quoteInt64(buf []byte, i int64) []byte {
	return fmt.Appendf(buf, "%q", i)
}

// Kind mismatches: the verb does not apply to the operand's kind.
func decFloat(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%d", f)
}

func decString(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%d", s)
}

func gComplex(buf []byte, c complex128) []byte {
	return fmt.Appendf(buf, "%g", c)
}

func gInt(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%g", i)
}

func tInt(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%t", i)
}

// A non-literal format proves nothing.
func varFormat(buf []byte, i int) []byte {
	format := "%d"
	return fmt.Appendf(buf, format, i)
}

// A spread call passes an unknown number of arguments.
func spread(buf []byte, args ...any) []byte {
	return fmt.Appendf(buf, "%d", args...)
}

// Two verbs / two operands are not the single-scalar shape.
func twoVerbs(buf []byte, a, b int) []byte {
	return fmt.Appendf(buf, "%d%d", a, b)
}

// A shadowed fmt is not the fmt package.
type fakeFmt struct{}

func (fakeFmt) Appendf(b []byte, format string, args ...any) []byte { return b }

func shadowedFmt(buf []byte, i int) []byte {
	fmt := fakeFmt{}
	return fmt.Appendf(buf, "%d", i)
}
