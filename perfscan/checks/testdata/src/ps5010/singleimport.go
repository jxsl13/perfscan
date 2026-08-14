package ps5010

import "bytes"

// A single non-parenthesized import declaration whose only user is the
// rewritten call: the whole spec is swapped for "strings" in place.
func single(s string) string {
	return string(bytes.ToLower([]byte(s))) // want `string\(bytes\.ToLower\(\[\]byte\(s\)\)\) copies`
}
