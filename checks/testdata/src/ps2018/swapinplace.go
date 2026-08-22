package ps2018

import (
	"bytes"
	"unicode"
)

// The spec after "bytes" already sorts after "strings", so the orphaned
// bytes spec is swapped for "strings" in place and the group stays
// gofmt-sorted.
func swapInPlace(s string) (string, bool) {
	t := string(bytes.Repeat([]byte(s), 4)) // want `string\(bytes\.Repeat\(\[\]byte\(s\), n\)\) copies`
	ok := len(t) > 0 && unicode.IsLetter(rune(t[0]))
	return t, ok
}
