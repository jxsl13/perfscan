package ps5082dot

import (
	. "strings"
	"unicode/utf8"
)

func dotImportStaysSilent(value string) bool {
	return utf8.ValidString(Clone(value))
}
