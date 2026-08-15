package ps2018

import (
	"bytes"
	str "strings"
)

// The file imports strings under an alias: the rewrite reuses it, and
// the orphaned bytes spec is removed.
func aliased(s string) string {
	return str.ToUpper(string(bytes.Repeat([]byte(s), 8))) // want `string\(bytes\.Repeat\(\[\]byte\(s\), n\)\) copies`
}
