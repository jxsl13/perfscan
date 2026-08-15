package ps5034

import (
	"strings"
	"unicode"
)

// This file keeps another unicode reference (IsDigit), so the rewrites
// do NOT orphan the import and no import surgery happens.
func keepImport(s string) string {
	if len(s) > 0 && unicode.IsDigit(rune(s[0])) {
		s = s[1:]
	}
	return strings.TrimFunc(s, unicode.IsSpace) // want `strings\.TrimFunc\(s, unicode\.IsSpace\) decodes`
}
