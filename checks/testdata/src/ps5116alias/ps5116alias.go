package ps5116alias

import (
	data "bytes"
	text "strings"
	u "unicode/utf8"
)

func validate(s string, b []byte) (bool, bool) {
	return u.ValidString(text.ToValidUTF8(s, "?")), // want `utf8.ValidString validates strings.ToValidUTF8 output whose replacement already guarantees valid UTF-8; the result is always true`
		u.Valid(data.ToValidUTF8(b, []byte("?"))) // want `utf8.Valid validates bytes.ToValidUTF8 output whose replacement already guarantees valid UTF-8; the result is always true`
}
