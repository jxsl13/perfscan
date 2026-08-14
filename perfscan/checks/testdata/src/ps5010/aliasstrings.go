package ps5010

import (
	"bytes"
	str "strings"
)

// The file imports strings under an alias: the rewrite reuses it, and
// the orphaned bytes spec is removed.
func aliased(s string) string {
	return str.TrimSpace(string(bytes.ToLower([]byte(s)))) // want `string\(bytes\.ToLower\(\[\]byte\(s\)\)\) copies`
}
