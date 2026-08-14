package ps5006

import (
	"bytes"
	"strings"
)

// The file keeps ANOTHER bytes reference and already imports strings, so
// the fixes leave the import block untouched.
func trimForms(s string, b []byte) []string {
	// A nested match in the OPERAND: both sites are rewritten (the
	// operand of the outer site is kept verbatim, so the inner rewrite
	// composes with it).
	twice := string(bytes.TrimPrefix([]byte(string(bytes.TrimPrefix([]byte(s), []byte("(")))), []byte("["))) // want `string\(bytes\.TrimPrefix\(\[\]byte\(s\), \[\]byte\(prefix\)\)\) copies` `string\(bytes\.TrimPrefix\(\[\]byte\(s\), \[\]byte\(prefix\)\)\) copies`

	// A match nested in the PREFIX argument is rewritten too: the prefix
	// expression is kept verbatim, so the inner rewrite composes.
	nestedPre := string(bytes.TrimPrefix([]byte(s), []byte(string(bytes.TrimSuffix([]byte(s), []byte(" ")))))) // want `string\(bytes\.TrimPrefix\(\[\]byte\(s\), \[\]byte\(prefix\)\)\) copies` `string\(bytes\.TrimSuffix\(\[\]byte\(s\), \[\]byte\(suffix\)\)\) copies`

	// Extra parentheses around the trim call and the conversions are
	// part of the replaced punctuation.
	parens := string((bytes.TrimSuffix(([]byte(s)), ([]byte("/"))))) // want `string\(bytes\.TrimSuffix\(\[\]byte\(s\), \[\]byte\(suffix\)\)\) copies`

	// Untyped constant operands are plain strings too; neither form is a
	// constant expression, so nothing changes at compile time.
	konst := string(bytes.TrimPrefix([]byte("__padded__"), []byte("__"))) // want `string\(bytes\.TrimPrefix\(\[\]byte\(s\), \[\]byte\(prefix\)\)\) copies`

	return []string{twice, nestedPre, parens, konst, string(bytes.ToLower(b)), strings.ToUpper(s)}
}
