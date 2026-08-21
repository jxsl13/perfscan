package ps5005

import (
	"bytes"
	str "strings"
)

// The file imports strings under an alias: the rewrite reuses it, and
// the orphaned bytes spec is removed.
func aliased(s string) string {
	return str.ToUpper(string(bytes.Trim([]byte(s), "*"))) // want `string\(bytes\.Trim\(\[\]byte\(s\), cutset\)\) copies`
}
