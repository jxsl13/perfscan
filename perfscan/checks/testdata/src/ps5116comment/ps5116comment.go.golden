package ps5116comment

import (
	"strings"
	"unicode/utf8"
)

func retained(payload string) bool {
	return utf8.ValidString( /* preserve validation rationale */ strings.ToValidUTF8(payload, "?")) // want `utf8.ValidString validates strings.ToValidUTF8 output whose replacement already guarantees valid UTF-8; the result is always true`
}
