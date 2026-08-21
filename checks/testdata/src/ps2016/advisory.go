package ps2016

import (
	"bytes"
	"unicode"
)

// The name strings is owned by a local at the call site: the fix cannot
// qualify the callee there, so the report stays advisory and the file is
// left untouched (no import edits either — nothing is fixable).
func shadowedStrings(s string) string {
	strings := []string{s}
	_ = strings
	return string(bytes.TrimFunc([]byte(s), unicode.IsSpace)) // want `string\(bytes\.TrimFunc\(\[\]byte\(s\), f\)\) copies`
}

// A comment inside the replaced punctuation would be silently destroyed
// by the rewrite — advisory.
func commentedInside(s string) string {
	return string(bytes.TrimFunc( /* keep me */ []byte(s), unicode.IsSpace)) // want `string\(bytes\.TrimFunc\(\[\]byte\(s\), f\)\) copies`
}

func commentedBetween(s string) string {
	return string(bytes.TrimLeftFunc([]byte(s) /* mid */, unicode.IsSpace)) // want `string\(bytes\.TrimLeftFunc\(\[\]byte\(s\), f\)\) copies`
}

func commentedAfter(s string) string {
	return string(bytes.TrimRightFunc([]byte(s), unicode.IsSpace /* tail */)) // want `string\(bytes\.TrimRightFunc\(\[\]byte\(s\), f\)\) copies`
}
