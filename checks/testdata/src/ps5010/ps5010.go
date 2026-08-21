package ps5010

import (
	"bytes"
	"fmt"
)

// These calls are the file's ONLY bytes references, so the fix also
// drops the orphaned bytes import and inserts strings at its sorted
// position ("fmt" orders between "bytes" and "strings", so an in-place
// swap would unsort the group).
func upperBasic(s string) string {
	return string(bytes.ToUpper([]byte(s))) // want `string\(bytes\.ToUpper\(\[\]byte\(s\)\)\) copies`
}

// All the siblings are matched and rewritten to their strings twin —
// including the deprecated Title, whose strings twin is deprecated in
// exactly the same way, so the rewrite does not change the code's
// deprecation posture.
func caseSiblings(s string) (string, string, string) {
	l := string(bytes.ToLower([]byte(s))) // want `string\(bytes\.ToLower\(\[\]byte\(s\)\)\) copies`
	t := string(bytes.ToTitle([]byte(s))) // want `string\(bytes\.ToTitle\(\[\]byte\(s\)\)\) copies`
	w := string(bytes.Title([]byte(s)))   // want `string\(bytes\.Title\(\[\]byte\(s\)\)\) copies`
	return l, t, w
}

// The operand is kept byte-verbatim, however it is spelled: a field
// selector, and a compound expression with a call (evaluated exactly
// once in both forms).
func upperVerbatim(w struct{ line string }, f func() string) {
	fmt.Println(string(bytes.ToUpper([]byte(w.line))))    // want `string\(bytes\.ToUpper\(\[\]byte\(s\)\)\) copies`
	fmt.Println(string(bytes.ToLower([]byte("x" + f())))) // want `string\(bytes\.ToLower\(\[\]byte\(s\)\)\) copies`
}
