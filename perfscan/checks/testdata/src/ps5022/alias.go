package ps5022

import (
	buf "bytes"
	str "strings"
)

// An aliased strings/bytes import keeps its qualifier verbatim; only the
// selected name, the wrapped literal, and (for ContainsAny) the appended
// comparison change — IndexByte lives in the same package, so no import
// surgery is ever needed.
func aliased(s string, b []byte) bool {
	i := str.IndexAny(s, "@") // want `strings\.IndexAny of the one-ASCII-byte cutset "@" pays the cutset dispatch`
	j := buf.IndexAny(b, "#") // want `bytes\.IndexAny of the one-ASCII-byte cutset "#" pays the cutset dispatch`
	return i+j >= 0 && buf.ContainsAny(b, "&") // want `bytes\.ContainsAny of the one-ASCII-byte cutset "&" routes a single byte`
}
