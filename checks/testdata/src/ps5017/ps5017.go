package ps5017

import (
	"bytes"
	"unicode"
)

// The mapping arguments are this file's ONLY unicode references, so the
// fix also drops the orphaned unicode import (the runner never prunes
// imports itself). The bytes import stays: the rewrite keeps the
// qualifier and merely renames Map.
func upperBasic(b []byte) []byte {
	return bytes.Map(unicode.ToUpper, b) // want `bytes\.Map\(unicode\.ToUpper, b\) pays`
}

func lowerBasic(b []byte) []byte {
	return bytes.Map(unicode.ToLower, b) // want `bytes\.Map\(unicode\.ToLower, b\) pays`
}

// The haystack is kept byte-verbatim, however it is spelled: a field
// selector, a compound expression with a call (evaluated exactly once
// in both forms), and a named type whose underlying type is []byte
// (assignable to both parameters, so the rewrite stays legal).
type payload []byte

func verbatim(w struct{ buf []byte }, f func() []byte, p payload) {
	_ = bytes.Map(unicode.ToUpper, w.buf)             // want `bytes\.Map\(unicode\.ToUpper, b\) pays`
	_ = bytes.Map(unicode.ToLower, append(f(), '!'))  // want `bytes\.Map\(unicode\.ToLower, b\) pays`
	_ = bytes.Map(unicode.ToUpper, p)                 // want `bytes\.Map\(unicode\.ToUpper, b\) pays`
	_ = bytes.Map((unicode.ToLower), w.buf)           // want `bytes\.Map\(unicode\.ToLower, b\) pays`
}

// A nested match sits inside the kept haystack span: both sites are
// rewritten (the edits never overlap).
func nested(b []byte) []byte {
	return bytes.Map(unicode.ToUpper, bytes.Map(unicode.ToLower, b)) // want `bytes\.Map\(unicode\.ToUpper, b\) pays` `bytes\.Map\(unicode\.ToLower, b\) pays`
}
