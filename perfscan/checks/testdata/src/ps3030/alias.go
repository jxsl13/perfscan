package ps3030

import (
	"bytes"
	u "unicode"
)

// An aliased unicode import still pins the predicate by type info; the
// whole ", u.IsSpace" tail is deleted. The keeper below holds another
// u reference, so the import survives the rewrite.
func aliasedUnicode(b []byte) [][]byte {
	return bytes.FieldsFunc(b, u.IsSpace) // want `bytes\.FieldsFunc\(b, unicode\.IsSpace\) decodes every rune`
}

var _ = u.IsPunct
