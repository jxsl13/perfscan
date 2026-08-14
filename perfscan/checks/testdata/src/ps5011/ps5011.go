package ps5011

import (
	"bytes"
	"fmt"
)

// These calls are the file's ONLY bytes references, so the fix also
// drops the orphaned bytes import and inserts strings at its sorted
// position ("fmt" orders between "bytes" and "strings", so an in-place
// swap would unsort the group).
func replaceBasic(s, old, new string) string {
	return string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new))) // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`
}

// Both siblings are matched and rewritten to their strings twin; the n
// argument of Replace is kept verbatim.
func replaceSiblings(s string, n int) (string, string) {
	all := string(bytes.ReplaceAll([]byte(s), []byte("-"), []byte("_")))     // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`
	some := string(bytes.Replace([]byte(s), []byte("."), []byte("/"), n+1)) // want `string\(bytes\.Replace\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\), n\)\) copies`
	return all, some
}

// The operands are kept byte-verbatim, however they are spelled: field
// selectors, compound expressions with calls, and calls producing the
// operands (each evaluated exactly once in both forms).
func replaceVerbatim(w struct{ line, from, to string }, f func() string) {
	fmt.Println(string(bytes.ReplaceAll([]byte(w.line), []byte(w.from), []byte(w.to))))    // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`
	fmt.Println(string(bytes.Replace([]byte("--"+f()), []byte(f()), []byte("-"+f()), 1))) // want `string\(bytes\.Replace\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\), n\)\) copies`
}
