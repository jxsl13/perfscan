package ps5005

import "bytes"

// A single non-parenthesized import declaration whose only user is the
// rewritten call: the whole spec is swapped for "strings" in place.
func single(s string) string {
	return string(bytes.TrimLeft([]byte(s), "0")) // want `string\(bytes\.TrimLeft\(\[\]byte\(s\), cutset\)\) copies`
}
