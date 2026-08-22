package ps2012

import "bytes"

// The name strings is owned by a local at the call site: the fix cannot
// qualify the callee there, so the report stays advisory and the file is
// left untouched (no import edits either — nothing is fixable).
func shadowedStrings(s string) string {
	strings := []string{s}
	_ = strings
	return string(bytes.TrimSpace([]byte(s))) // want `string\(bytes\.TrimSpace\(\[\]byte\(s\)\)\) copies`
}

// A comment inside the replaced punctuation would be silently destroyed
// by the rewrite — advisory.
func commentedInside(s string) string {
	return string(bytes.TrimSpace( /* keep me */ []byte(s))) // want `string\(bytes\.TrimSpace\(\[\]byte\(s\)\)\) copies`
}

func commentedAfter(s string) string {
	return string(bytes.TrimSpace([]byte(s) /* tail */)) // want `string\(bytes\.TrimSpace\(\[\]byte\(s\)\)\) copies`
}
