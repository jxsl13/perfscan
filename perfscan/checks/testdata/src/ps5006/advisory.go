package ps5006

import "bytes"

// The name strings is owned by a local at the call site: the fix cannot
// qualify the callee there, so the report stays advisory and the file is
// left untouched (no import edits either — nothing is fixable).
func shadowedStrings(s string) string {
	strings := []string{s}
	_ = strings
	return string(bytes.TrimPrefix([]byte(s), []byte("#"))) // want `string\(bytes\.TrimPrefix\(\[\]byte\(s\), \[\]byte\(prefix\)\)\) copies`
}

// A comment inside the replaced punctuation would be silently destroyed
// by the rewrite — advisory.
func commentedInside(s string) string {
	return string(bytes.TrimPrefix( /* keep me */ []byte(s), []byte("#"))) // want `string\(bytes\.TrimPrefix\(\[\]byte\(s\), \[\]byte\(prefix\)\)\) copies`
}

func commentedBetween(s string) string {
	return string(bytes.TrimPrefix([]byte(s) /* mid */, []byte("#"))) // want `string\(bytes\.TrimPrefix\(\[\]byte\(s\), \[\]byte\(prefix\)\)\) copies`
}

func commentedAfter(s string) string {
	return string(bytes.TrimSuffix([]byte(s), []byte("#" /* tail */))) // want `string\(bytes\.TrimSuffix\(\[\]byte\(s\), \[\]byte\(suffix\)\)\) copies`
}
