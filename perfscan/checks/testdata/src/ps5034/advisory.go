package ps5034

import (
	"strings"
	"unicode"
)

// A comment inside the deleted span (from the source argument through
// the predicate to the closing parenthesis) would be silently
// destroyed — that report stays advisory and its call is left
// untouched.
func commentedInside(s string) string {
	return strings.TrimFunc(s /* keep me */, unicode.IsSpace) // want `strings\.TrimFunc\(s, unicode\.IsSpace\) decodes`
}

func commentedTail(s string) string {
	return strings.TrimFunc(s, unicode.IsSpace /* tail */) // want `strings\.TrimFunc\(s, unicode\.IsSpace\) decodes`
}

// The advisory sites above keep referencing unicode, so this file's
// fixable site must NOT drop the import: the orphan accounting counts
// only the references the applied fixes actually delete.
func fixedAlongside(s string) string {
	return strings.TrimFunc(s, unicode.IsSpace) // want `strings\.TrimFunc\(s, unicode\.IsSpace\) decodes`
}
