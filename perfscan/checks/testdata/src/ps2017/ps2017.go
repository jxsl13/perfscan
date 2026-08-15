package ps2017

import (
	"bytes"
	"fmt"
	"unicode"
)

// These calls are the file's ONLY bytes references, so the fix also
// drops the orphaned bytes import and inserts strings at its sorted
// position ("fmt" orders between "bytes" and "strings", so an in-place
// swap would unsort the group).
func mapBasic(s string) string {
	return string(bytes.Map(unicode.ToUpper, []byte(s))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}

// Both arguments are kept byte-verbatim, however they are spelled: a
// named func value, a multi-line closure with rune-dropping, a field
// selector and a compound operand with a call (each evaluated exactly
// once in both forms), and an untyped constant operand.
func mapVerbatim(w struct{ line string }, next func() string) {
	shift := func(r rune) rune { return r + 1 }
	fmt.Println(string(bytes.Map(shift, []byte(w.line)))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
	fmt.Println(string(bytes.Map(func(r rune) rune { // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
		if r == 'x' {
			return -1
		}
		return unicode.ToLower(r)
	}, []byte("x"+next()))))
	fmt.Println(string(bytes.Map(shift, []byte("constant")))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}

// Parenthesized shapes: the inner call and the conversion target may be
// wrapped in parentheses; the rewrite still only replaces punctuation.
func mapParens(s string) string {
	return string((bytes.Map(unicode.ToTitle, ([]byte)(s)))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}
