package ps2017

import (
	"bytes"
	"unicode"
)

// The spec after "bytes" already sorts after "strings", so the orphaned
// bytes spec is swapped for "strings" in place and the group stays
// gofmt-sorted.
func swapInPlace(s string) string {
	return string(bytes.Map(unicode.ToLower, []byte(s))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}
