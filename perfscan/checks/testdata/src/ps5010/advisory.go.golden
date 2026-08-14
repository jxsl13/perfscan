package ps5010

import "bytes"

// The name strings is owned by a local at the call site: the fix cannot
// qualify the callee there, so the report stays advisory and the file is
// left untouched (no import edits either — nothing is fixable).
func shadowedStrings(s string) string {
	strings := []string{s}
	_ = strings
	return string(bytes.ToUpper([]byte(s))) // want `string\(bytes\.ToUpper\(\[\]byte\(s\)\)\) copies`
}

// A comment inside the replaced punctuation would be silently destroyed
// by the rewrite — advisory.
func commentedInside(s string) string {
	return string(bytes.ToUpper( /* keep me */ []byte(s))) // want `string\(bytes\.ToUpper\(\[\]byte\(s\)\)\) copies`
}

func commentedAfter(s string) string {
	return string(bytes.ToLower([]byte(s) /* tail */)) // want `string\(bytes\.ToLower\(\[\]byte\(s\)\)\) copies`
}
