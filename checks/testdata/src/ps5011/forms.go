package ps5011

import (
	"bytes"
	"strings"
)

// The file keeps ANOTHER bytes reference and already imports strings, so
// the fixes leave the import block untouched.
func replaceForms(s string, b []byte) []string {
	// A nested match: both sites are rewritten (the operand of the outer
	// site is kept verbatim, so the inner rewrite composes with it).
	twice := string(bytes.ReplaceAll([]byte(string(bytes.ReplaceAll([]byte(s), []byte("("), []byte("[")))), []byte(")"), []byte("]"))) // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies` `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`

	// A match nested in the old/new arguments is rewritten too: those
	// operand expressions are kept verbatim, so the inner rewrite
	// composes.
	nestedOld := string(bytes.ReplaceAll([]byte(s), []byte(string(bytes.ReplaceAll([]byte(s), []byte(" "), []byte("")))), []byte("_"))) // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies` `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`

	// Extra parentheses around the replace call and the conversions are
	// part of the replaced punctuation.
	parens := string((bytes.Replace(([]byte(s)), ([]byte("//")), []byte("/"), -1))) // want `string\(bytes\.Replace\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\), n\)\) copies`

	// Untyped constant operands are plain strings too; neither form is a
	// constant expression, so nothing changes at compile time.
	konst := string(bytes.ReplaceAll([]byte("a-b-c"), []byte("-"), []byte("+"))) // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`

	// A verbatim compound n expression is passed through untouched.
	capped := string(bytes.Replace([]byte(s), []byte("x"), []byte("y"), len(s)/2)) // want `string\(bytes\.Replace\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\), n\)\) copies`

	return []string{twice, nestedOld, parens, konst, capped, string(bytes.ToLower(b)), strings.ToUpper(s)}
}
