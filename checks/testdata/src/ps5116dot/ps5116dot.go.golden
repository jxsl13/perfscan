package ps5116dot

import (
	. "strings"
	u "unicode/utf8"
)

func excluded(payload string) bool {
	return u.ValidString(ToValidUTF8(payload, "?"))
}
