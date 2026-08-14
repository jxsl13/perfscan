package ps5005

import (
	"bytes"
	"strings"
)

// The file keeps ANOTHER bytes reference and already imports strings, so
// the fixes leave the import block untouched.
func trimForms(s string, b []byte) []string {
	// A nested match: both sites are rewritten (the operand of the outer
	// site is kept verbatim, so the inner rewrite composes with it).
	twice := string(bytes.Trim([]byte(string(bytes.Trim([]byte(s), "()"))), "[]")) // want `string\(bytes\.Trim\(\[\]byte\(s\), cutset\)\) copies` `string\(bytes\.Trim\(\[\]byte\(s\), cutset\)\) copies`

	// A match nested in the CUTSET argument is rewritten too: the cutset
	// expression is kept verbatim, so the inner rewrite composes.
	nestedCut := string(bytes.Trim([]byte(s), string(bytes.TrimLeft([]byte(s), " ")))) // want `string\(bytes\.Trim\(\[\]byte\(s\), cutset\)\) copies` `string\(bytes\.TrimLeft\(\[\]byte\(s\), cutset\)\) copies`

	// Extra parentheses around the trim call and the conversion are part
	// of the replaced punctuation.
	parens := string((bytes.TrimRight(([]byte(s)), "/"))) // want `string\(bytes\.TrimRight\(\[\]byte\(s\), cutset\)\) copies`

	// An untyped constant operand is a plain string too; neither form is
	// a constant expression, so nothing changes at compile time.
	konst := string(bytes.Trim([]byte("__padded__"), "_")) // want `string\(bytes\.Trim\(\[\]byte\(s\), cutset\)\) copies`

	return []string{twice, nestedCut, parens, konst, string(bytes.ToLower(b)), strings.ToUpper(s)}
}
