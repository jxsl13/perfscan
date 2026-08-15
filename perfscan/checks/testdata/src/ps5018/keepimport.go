package ps5018

import (
	"strings"
	"unicode"
)

// This file keeps another unicode reference (IsSpace), so the rewrites
// do NOT orphan the import and no import surgery happens.
func keepImport(s string) string {
	if len(s) > 0 && unicode.IsSpace(rune(s[0])) {
		s = s[1:]
	}
	return strings.Map(unicode.ToUpper, s) // want `strings\.Map\(unicode\.ToUpper, s\) pays`
}
