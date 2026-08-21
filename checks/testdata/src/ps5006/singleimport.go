package ps5006

import "bytes"

// A single non-parenthesized import declaration whose only user is the
// rewritten call: the whole spec is swapped for "strings" in place.
func single(s string) string {
	return string(bytes.TrimPrefix([]byte(s), []byte("0x"))) // want `string\(bytes\.TrimPrefix\(\[\]byte\(s\), \[\]byte\(prefix\)\)\) copies`
}
