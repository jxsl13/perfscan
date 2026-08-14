package ps2012

import (
	"bytes"
	"fmt"
)

// These three calls are the file's ONLY bytes references, so the fix also
// drops the orphaned bytes import and inserts strings at its sorted
// position ("fmt" orders between "bytes" and "strings", so an in-place
// swap would unsort the group).
func trimBasic(s string) string {
	return string(bytes.TrimSpace([]byte(s))) // want `string\(bytes\.TrimSpace\(\[\]byte\(s\)\)\) copies`
}

// The operand expression is kept byte-verbatim, however it is spelled:
// a field selector and a compound expression with a call (evaluated
// exactly once in both forms).
func trimVerbatim(w struct{ line string }, f func() string) {
	fmt.Println(string(bytes.TrimSpace([]byte(w.line))))        // want `string\(bytes\.TrimSpace\(\[\]byte\(s\)\)\) copies`
	fmt.Println(string(bytes.TrimSpace([]byte("  " + f())))) // want `string\(bytes\.TrimSpace\(\[\]byte\(s\)\)\) copies`
}
