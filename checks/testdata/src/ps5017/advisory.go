package ps5017

import (
	"bytes"
	"unicode"
)

// A comment inside the deleted span (mapping argument through the
// comma) would be silently destroyed — that report stays advisory and
// its call is left untouched.
func commentedInside(b []byte) []byte {
	return bytes.Map(unicode.ToUpper /* keep me */, b) // want `bytes\.Map\(unicode\.ToUpper, b\) pays`
}

// The advisory site above keeps referencing unicode, so this file's
// fixable site must NOT drop the import: the orphan accounting counts
// only the references the applied fixes actually delete.
func fixedAlongside(b []byte) []byte {
	return bytes.Map(unicode.ToLower, b) // want `bytes\.Map\(unicode\.ToLower, b\) pays`
}
