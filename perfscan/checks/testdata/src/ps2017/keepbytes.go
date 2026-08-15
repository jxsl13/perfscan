package ps2017

import (
	"bytes"
	"unicode"
)

// The file keeps another bytes reference after the rewrite, so the
// bytes import stays and only strings is added.
func keepBytes(s string, b []byte) (string, []byte) {
	t := string(bytes.Map(unicode.ToUpper, []byte(s))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
	return t, bytes.TrimSpace(b)
}

// A nested match: both sites are rewritten (the operand of the outer
// site is kept verbatim, so the inner rewrite composes with it).
func nested(s string) string {
	return string(bytes.Map(unicode.ToUpper, []byte(string(bytes.Map(unicode.ToLower, []byte(s)))))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies` `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}
