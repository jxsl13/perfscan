package ps2139

import (
	"bufio"
	"bytes"
	"strings"
)

func builder(b *strings.Builder, r rune) {
	b.WriteString(string(r)) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
}

func buffer(b *bytes.Buffer, r rune) {
	b.WriteString(string(r)) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
}

// The results keep their exact values: on Builder/Buffer both calls return
// the encoded byte count and a nil error, so a used result is still fixed.
func resultUsed(b *strings.Builder, r rune) (int, error) {
	return b.WriteString(string(r)) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
}

// A value receiver works: the pointer-receiver WriteString already required
// the variable to be addressable, so WriteRune is reachable the same way.
func valueRecv(r rune) string {
	var sb strings.Builder
	sb.WriteString(string(r)) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
	return sb.String()
}

// The narrower int32-lossless widths are wrapped in a value-preserving
// rune(...) conversion; string(x) and string(rune(x)) agree for every value
// of these types.
func widths(b *strings.Builder, c byte, i8 int8, i16 int16, u16 uint16) {
	b.WriteString(string(c))   // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
	b.WriteString(string(i8))  // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
	b.WriteString(string(i16)) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
	b.WriteString(string(u16)) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
}

// A named rune type is not assignable to the rune parameter, so it is
// wrapped too — rune(namedRune) is the identity on the underlying int32.
type myRune rune

func named(b *bytes.Buffer, r myRune) {
	b.WriteString(string(r)) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
}

// Parentheses around the conversion survive the unwrap.
func parens(b *bytes.Buffer, r rune) {
	b.WriteString((string(r))) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
}

// A non-trivial operand expression is kept verbatim in place.
func exprOperand(b *strings.Builder, rs []rune, i int) {
	b.WriteString(string(rs[i+1])) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
}

// Promotion through an embedded std writer resolves both methods to the
// std pair — the fix still applies.
type wrapper struct {
	*bytes.Buffer
}

func embedded(w wrapper, r rune) {
	w.WriteString(string(r)) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
}

// --- advisory: reported but never rewritten ---

// bufio.Writer diverges on the flush-error path: WriteString can write a
// partial rune and report a partial count where WriteRune reports (0, err).
func buffered(w *bufio.Writer, r rune) {
	w.WriteString(string(r)) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer; bufio\.Writer diverges on the flush-error path \(WriteString reports a partial count where WriteRune reports 0\) — rewrite by hand if the error path allows it`
}

// A custom pair carries no proof the two methods agree.
type custom struct{}

func (custom) WriteString(s string) (int, error) { return len(s), nil }
func (custom) WriteRune(r rune) (int, error)     { return 0, nil }

func customPair(c custom, r rune) {
	c.WriteString(string(r)) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer; this WriteString/WriteRune pair is not the standard library's — verify WriteRune appends the identical UTF-8 encoding before rewriting`
}

// An interface pair is equally unproven.
type runeStringWriter interface {
	WriteString(s string) (int, error)
	WriteRune(r rune) (int, error)
}

func iface(w runeStringWriter, r rune) {
	w.WriteString(string(r)) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer; this WriteString/WriteRune pair is not the standard library's — verify WriteRune appends the identical UTF-8 encoding before rewriting`
}

// A comment inside the call would be destroyed by the rewrite: the report
// stays advisory.
func commented(b *strings.Builder, r rune) {
	b.WriteString(string(r) /* keep me */) // want `w\.WriteString\(string\(r\)\) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune\(r\) encodes straight into the buffer`
}

// --- guards: none of the following may be reported ---

// Wider integer types diverge under the rune() truncation the rewrite
// would need: string(x) yields U+FFFD for an out-of-range value where
// rune(x) truncates — never touched.
func wide(b *strings.Builder, i int, i64 int64, u32 uint32, u64 uint64, p uintptr) {
	b.WriteString(string(i))
	b.WriteString(string(i64))
	b.WriteString(string(u32))
	b.WriteString(string(u64))
	b.WriteString(string(p))
}

// A constant conversion is folded at compile time and allocates nothing.
func constant(b *strings.Builder) {
	const sep rune = ':'
	b.WriteString(string('a'))
	b.WriteString(string(sep))
}

// A plain string, a byte slice (PS2135's territory) and a rune slice are
// not single-rune conversions.
func otherArgs(b *strings.Builder, s string, bs []byte, rs []rune) {
	b.WriteString(s)
	b.WriteString(string(bs))
	b.WriteString(string(rs))
}

// A receiver without a WriteRune(rune) (int, error) method has nothing to
// rewrite to.
type strOnly struct{}

func (strOnly) WriteString(s string) (int, error) { return len(s), nil }

func noWriteRune(w strOnly, r rune) {
	w.WriteString(string(r))
}

// A pointer-receiver WriteRune is not in the method set of a value
// returned by a call: the rewrite would not even compile.
type mixed struct{}

func (mixed) WriteString(s string) (int, error) { return len(s), nil }
func (*mixed) WriteRune(r rune) (int, error)    { return 0, nil }

func makeMixed() mixed { return mixed{} }

func notAddressable(r rune) {
	makeMixed().WriteString(string(r))
}

// A function-typed field named WriteString is not a method call.
type fieldFunc struct {
	WriteString func(s string) (int, error)
}

func field(f fieldFunc, r rune) {
	f.WriteString(string(r))
}

// A shadowed string identifier is a call, not a conversion.
func shadowed(b *strings.Builder, r rune) {
	string := func(r rune) string { return "x" }
	b.WriteString(string(r))
}

// WriteString(string(r)) lexically inside the receiver's own WriteRune is
// the correct delegation — the rewrite would recurse forever.
type delegator struct {
	sb strings.Builder
}

func (d *delegator) WriteString(s string) (int, error) { return d.sb.WriteString(s) }
func (d *delegator) WriteRune(r rune) (int, error) {
	return d.WriteString(string(r))
}
