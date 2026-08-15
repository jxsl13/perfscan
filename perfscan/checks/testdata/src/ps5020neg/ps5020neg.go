package ps5020neg

type B []byte
type S string

func makeBytes() []byte { return nil }

// None of these may produce a diagnostic.
func negatives(dst []byte, bsl []byte, s string, rs []rune) ([]byte, []rune) {
	dst = append(dst, s...)            // already the direct form
	dst = append(dst, bsl...)          // plain slice spread
	dst = append(dst, []byte(bsl)...)  // operand is already []byte: a no-op conversion, not this pattern
	dst = append(dst, B(s)...)         // NAMED byte-slice conversion type is out of scope
	dst = append(dst, []byte{1, 2}...) // composite literal, not a conversion
	dst = append(dst, makeBytes()...)  // function call, not a conversion
	dst = append(dst, s[0], s[1])      // non-spread element appends
	dst = append(dst, []byte(nil)...)  // nil operand, not a string
	rs = append(rs, []rune(s)...)      // rune conversion: different element type
	return dst, rs
}

// A type-parameter operand is conservatively skipped even though its
// core type is string.
func genericOperand[T ~string](dst []byte, s T) []byte {
	return append(dst, []byte(s)...)
}

// A shadowed append is not the builtin and never matches.
func shadowed(dst []byte, s string) []byte {
	append := func(b []byte, extra ...byte) []byte { return b }
	return append(dst, []byte(s)...)
}
