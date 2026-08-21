package ps3030

import (
	"bytes"
	"unicode"
)

// The two predicates below are this file's ONLY unicode references:
// applying both fixes orphans the import, so the fix drops it.
func orphanOne(b []byte) [][]byte {
	return bytes.FieldsFunc(b, unicode.IsSpace) // want `bytes\.FieldsFunc\(b, unicode\.IsSpace\) decodes every rune`
}

func orphanTwo(b []byte) [][][]byte {
	return [][][]byte{
		bytes.FieldsFunc(b, unicode.IsSpace), // want `bytes\.FieldsFunc\(b, unicode\.IsSpace\) decodes every rune`
	}
}
