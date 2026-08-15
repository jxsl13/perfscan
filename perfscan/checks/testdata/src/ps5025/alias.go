package ps5025

import (
	buf "bytes"
	str "strings"
)

// An aliased strings/bytes import keeps its qualifier verbatim; only the
// selected name and the wrapped literal change — LastIndexByte lives in
// the same package, so no import surgery is ever needed.
func aliased(s string, b []byte) int {
	i := str.LastIndexAny(s, "@") // want `strings\.LastIndexAny of the one-ASCII-byte cutset "@" pays the cutset dispatch`
	j := buf.LastIndexAny(b, "#") // want `bytes\.LastIndexAny of the one-ASCII-byte cutset "#" pays the cutset dispatch`
	return i + j
}
