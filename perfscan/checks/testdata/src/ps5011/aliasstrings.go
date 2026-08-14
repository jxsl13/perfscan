package ps5011

import (
	"bytes"
	str "strings"
)

// The file imports strings under an alias: the rewrite reuses it, and
// the orphaned bytes spec is removed.
func aliased(s string) string {
	return str.ToUpper(string(bytes.ReplaceAll([]byte(s), []byte("*"), []byte("")))) // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`
}
