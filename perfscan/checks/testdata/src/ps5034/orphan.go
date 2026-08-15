package ps5034

import (
	"strings"
	"unicode"
)

// The two predicates below are this file's ONLY unicode references:
// applying both fixes orphans the import, so the fix drops it.
func orphanOne(s string) []string {
	return strings.FieldsFunc(s, unicode.IsSpace) // want `strings\.FieldsFunc\(s, unicode\.IsSpace\) decodes every rune`
}

func orphanTwo(s string) [][]string {
	return [][]string{
		strings.FieldsFunc(s, unicode.IsSpace), // want `strings\.FieldsFunc\(s, unicode\.IsSpace\) decodes every rune`
	}
}
