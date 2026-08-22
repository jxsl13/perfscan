package ps2016

import (
	"bytes"
	"fmt"
	"unicode"
)

// These calls are the file's ONLY bytes references, so the fix also
// drops the orphaned bytes import and inserts strings at its sorted
// position ("fmt" orders between "bytes" and "strings", so an in-place
// swap would unsort the group).
func trimBasic(s string) string {
	return string(bytes.TrimFunc([]byte(s), unicode.IsSpace)) // want `string\(bytes\.TrimFunc\(\[\]byte\(s\), f\)\) copies`
}

// All three siblings are matched and rewritten to their strings twin;
// the predicate is kept byte-verbatim whether it is a named func, a
// func literal, or a method value.
func trimSiblings(s string) (string, string) {
	l := string(bytes.TrimLeftFunc([]byte(s), unicode.IsDigit))                          // want `string\(bytes\.TrimLeftFunc\(\[\]byte\(s\), f\)\) copies`
	r := string(bytes.TrimRightFunc([]byte(s), func(r rune) bool { return r == '/' })) // want `string\(bytes\.TrimRightFunc\(\[\]byte\(s\), f\)\) copies`
	return l, r
}

// The operand and the predicate are kept byte-verbatim, however they
// are spelled: a field selector, a compound expression with a call, a
// method value, and a call producing the predicate (each evaluated
// exactly once in both forms).
type trimmer struct{ line string }

func (t trimmer) drop(r rune) bool { return r == '-' }

func trimVerbatim(w trimmer, f func() func(rune) bool) {
	fmt.Println(string(bytes.TrimFunc([]byte(w.line), w.drop)))   // want `string\(bytes\.TrimFunc\(\[\]byte\(s\), f\)\) copies`
	fmt.Println(string(bytes.TrimFunc([]byte("--"+w.line), f()))) // want `string\(bytes\.TrimFunc\(\[\]byte\(s\), f\)\) copies`
}
