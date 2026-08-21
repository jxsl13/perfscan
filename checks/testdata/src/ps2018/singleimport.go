package ps2018

import "bytes"

// A single non-parenthesized import declaration whose only user is the
// rewritten call: the whole spec is swapped for "strings" in place.
func single(s string) string {
	return string(bytes.Repeat([]byte(s+"|"), 16)) // want `string\(bytes\.Repeat\(\[\]byte\(s\), n\)\) copies`
}
