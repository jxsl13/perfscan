package ps5006

import (
	"bytes"
	"fmt"
)

// These calls are the file's ONLY bytes references, so the fix also
// drops the orphaned bytes import and inserts strings at its sorted
// position ("fmt" orders between "bytes" and "strings", so an in-place
// swap would unsort the group).
func trimBasic(s, p string) string {
	return string(bytes.TrimPrefix([]byte(s), []byte(p))) // want `string\(bytes\.TrimPrefix\(\[\]byte\(s\), \[\]byte\(prefix\)\)\) copies`
}

// Both siblings are matched and rewritten to their strings twin.
func trimSiblings(s string) (string, string) {
	pre := string(bytes.TrimPrefix([]byte(s), []byte("v=")))  // want `string\(bytes\.TrimPrefix\(\[\]byte\(s\), \[\]byte\(prefix\)\)\) copies`
	suf := string(bytes.TrimSuffix([]byte(s), []byte("\r\n"))) // want `string\(bytes\.TrimSuffix\(\[\]byte\(s\), \[\]byte\(suffix\)\)\) copies`
	return pre, suf
}

// The operand and the prefix are kept byte-verbatim, however they are
// spelled: a field selector, a compound expression with a call, and a
// call producing the prefix (each evaluated exactly once in both forms).
func trimVerbatim(w struct{ line, pre string }, f func() string) {
	fmt.Println(string(bytes.TrimPrefix([]byte(w.line), []byte(w.pre))))     // want `string\(bytes\.TrimPrefix\(\[\]byte\(s\), \[\]byte\(prefix\)\)\) copies`
	fmt.Println(string(bytes.TrimSuffix([]byte("--"+f()), []byte("-"+f())))) // want `string\(bytes\.TrimSuffix\(\[\]byte\(s\), \[\]byte\(suffix\)\)\) copies`
}
