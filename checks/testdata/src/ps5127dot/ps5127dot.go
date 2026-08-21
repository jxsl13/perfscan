package ps5127dot

import (
	. "strings"
	. "unicode/utf8"
)

func sanitize(value string) string {
	if !ValidString(value) {
		return ToValidUTF8(value, "?")
	}
	return value
}
