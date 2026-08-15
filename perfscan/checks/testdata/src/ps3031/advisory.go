package ps3031

import (
	"bytes"
	"unicode"
)

// A comment inside the deleted span (from the source argument through
// the predicate to the closing parenthesis) would be silently
// destroyed — that report stays advisory and its call is left
// untouched.
func commentedInside(b []byte) []byte {
	return bytes.TrimFunc(b /* keep me */, unicode.IsSpace) // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
}

func commentedTail(b []byte) []byte {
	return bytes.TrimFunc(b, unicode.IsSpace /* tail */) // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
}

// The advisory sites above keep referencing unicode, so this file's
// fixable site must NOT drop the import: the orphan accounting counts
// only the references the applied fixes actually delete.
func fixedAlongside(b []byte) []byte {
	return bytes.TrimFunc(b, unicode.IsSpace) // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
}
