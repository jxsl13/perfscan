package ps5011

import "bytes"

// A single non-parenthesized import declaration whose only user is the
// rewritten call: the whole spec is swapped for "strings" in place.
func single(s string, n int) string {
	return string(bytes.Replace([]byte(s), []byte("0"), []byte("O"), n)) // want `string\(bytes\.Replace\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\), n\)\) copies`
}
