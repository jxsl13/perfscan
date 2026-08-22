package ps5010

import (
	"bytes"
	"strings"
)

// The file keeps ANOTHER bytes reference and already imports strings, so
// the fixes leave the import block untouched.
func caseForms(s string, b []byte) []string {
	// A nested match: both sites are rewritten (the operand of the outer
	// site is kept verbatim, so the inner rewrite composes with it).
	twice := string(bytes.ToUpper([]byte(string(bytes.ToLower([]byte(s)))))) // want `string\(bytes\.ToUpper\(\[\]byte\(s\)\)\) copies` `string\(bytes\.ToLower\(\[\]byte\(s\)\)\) copies`

	// Extra parentheses around the case call and the conversion are part
	// of the replaced punctuation.
	parens := string((bytes.ToTitle(([]byte(s))))) // want `string\(bytes\.ToTitle\(\[\]byte\(s\)\)\) copies`

	// An untyped constant operand is a plain string too; neither form is
	// a constant expression, so nothing changes at compile time.
	konst := string(bytes.ToUpper([]byte("mixed Case"))) // want `string\(bytes\.ToUpper\(\[\]byte\(s\)\)\) copies`

	return []string{twice, parens, konst, string(bytes.TrimSpace(b)), strings.ToUpper(s)}
}
