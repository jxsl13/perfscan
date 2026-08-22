package ps2017

import (
	"bytes"
	str "strings"
)

// The file imports strings under an alias: the rewrite reuses it, and
// the orphaned bytes spec is removed.
func aliased(s string) string {
	return str.TrimSpace(string(bytes.Map(func(r rune) rune { return r }, []byte(s)))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}
