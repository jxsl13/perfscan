package ps2038

import "unicode/utf8"

func use(rune, int) {}

// Reverse direction — the real win: the whole slice is copied into a
// throwaway string to decode ONE rune.
func first(b []byte) (rune, int) {
	r, size := utf8.DecodeRuneInString(string(b)) // want `utf8\.DecodeRune\(b\) decodes the identical \(r, size\) in place`
	return r, size
}

func last(b []byte) (rune, int) {
	r, size := utf8.DecodeLastRuneInString(string(b)) // want `utf8\.DecodeLastRune\(b\) decodes the identical \(r, size\) in place`
	return r, size
}

// Forward direction: never a regression, a real win off current gc.
func firstForward(s string) (rune, int) {
	r, size := utf8.DecodeRune([]byte(s)) // want `utf8\.DecodeRuneInString\(s\) decodes the identical \(r, size\) in place`
	return r, size
}

func lastForward(s string) (rune, int) {
	r, size := utf8.DecodeLastRune([]byte(s)) // want `utf8\.DecodeLastRuneInString\(s\) decodes the identical \(r, size\) in place`
	return r, size
}

// []uint8 spells the identical []byte type.
func uint8Spelling(s string) (rune, int) {
	return utf8.DecodeRune([]uint8(s)) // want `utf8\.DecodeRuneInString\(s\) decodes the identical \(r, size\) in place`
}

// An untyped string constant operand defaults to string.
func constOperand() (rune, int) {
	return utf8.DecodeRune([]byte("héllo")) // want `utf8\.DecodeRuneInString\("héllo"\) decodes the identical \(r, size\) in place`
}

// Redundant parentheses around the conversion are deleted with it, and
// a parenthesized operand carries over verbatim.
func parens(b []byte) (rune, int) {
	return utf8.DecodeLastRuneInString((string((b)))) // want `utf8\.DecodeLastRune\(\(b\)\) decodes the identical \(r, size\) in place`
}

// A side-effecting operand is evaluated exactly once in both spellings
// and carries over byte-verbatim.
func sideEffect(next func() []byte) (rune, int) {
	return utf8.DecodeRuneInString(string(next())) // want `utf8\.DecodeRune\(next\(\)\) decodes the identical \(r, size\) in place`
}

// Selector and index operands carry over verbatim.
type holder struct{ raw []byte }

func selector(h holder, rows [][]byte) {
	use(utf8.DecodeRuneInString(string(h.raw)))     // want `utf8\.DecodeRune\(h\.raw\) decodes the identical \(r, size\) in place`
	use(utf8.DecodeLastRuneInString(string(rows[0]))) // want `utf8\.DecodeLastRune\(rows\[0\]\) decodes the identical \(r, size\) in place`
}

// Reported but NOT fixed: a comment inside the deleted conversion
// syntax would be destroyed by the edits.
func commented(b []byte) (rune, int) {
	return utf8.DecodeRuneInString(string( // keep me // want `a comment inside the conversion syntax withholds the automatic fix`
		b))
}
