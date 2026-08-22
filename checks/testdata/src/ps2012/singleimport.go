package ps2012

import "bytes"

// A single non-parenthesized import declaration whose only user is the
// rewritten call: the whole spec is swapped for "strings" in place.
func single(s string) string {
	return string(bytes.TrimSpace([]byte(s))) // want `string\(bytes\.TrimSpace\(\[\]byte\(s\)\)\) copies`
}
