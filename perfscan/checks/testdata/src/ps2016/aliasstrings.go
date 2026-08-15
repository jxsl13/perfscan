package ps2016

import (
	"bytes"
	str "strings"
	"unicode"
)

// The file imports strings under an alias: the rewrite reuses it, and
// the orphaned bytes spec is removed.
func aliased(s string) string {
	return str.ToUpper(string(bytes.TrimFunc([]byte(s), unicode.IsSpace))) // want `string\(bytes\.TrimFunc\(\[\]byte\(s\), f\)\) copies`
}
