package ps5127alias

import (
	text "strings"
	encoding "unicode/utf8"
)

func sanitize(value string) string {
	// want +1 `utf8.ValidString scans before strings.ToValidUTF8 repeats validation and repair`
	if !encoding.ValidString(value) {
		return text.ToValidUTF8(value, "?")
	}
	return value
}
