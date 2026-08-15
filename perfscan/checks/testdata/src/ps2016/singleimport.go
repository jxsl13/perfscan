package ps2016

import "bytes"

// A single non-parenthesized import declaration whose only user is the
// rewritten call: the whole spec is swapped for "strings" in place (the
// predicate is a func literal, so no unicode import is involved).
func single(s string) string {
	return string(bytes.TrimLeftFunc([]byte(s), func(r rune) bool { return r == '0' })) // want `string\(bytes\.TrimLeftFunc\(\[\]byte\(s\), f\)\) copies`
}
