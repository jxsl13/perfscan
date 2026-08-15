package ps3030

import (
	"bytes"
	"unicode"
)

// One advisory site (comment in the deleted scaffolding) plus one
// fixable site: the advisory site KEEPS its unicode reference, so the
// import must survive even though the fixable rewrite removes one.
func mixedFixable(b []byte) [][]byte {
	return bytes.FieldsFunc(b, unicode.IsSpace) // want `bytes\.FieldsFunc\(b, unicode\.IsSpace\) decodes every rune`
}

func mixedAdvisory(b []byte) [][]byte {
	return bytes.FieldsFunc(b /* whitespace */, unicode.IsSpace) // want `bytes\.FieldsFunc\(b, unicode\.IsSpace\) decodes every rune`
}
