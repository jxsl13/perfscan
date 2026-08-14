package ps5005

import "bytes"

// The name strings is owned by a local at the call site: the fix cannot
// qualify the callee there, so the report stays advisory and the file is
// left untouched (no import edits either — nothing is fixable).
func shadowedStrings(s string) string {
	strings := []string{s}
	_ = strings
	return string(bytes.Trim([]byte(s), " ")) // want `string\(bytes\.Trim\(\[\]byte\(s\), cutset\)\) copies`
}

// A comment inside the replaced punctuation would be silently destroyed
// by the rewrite — advisory.
func commentedInside(s string) string {
	return string(bytes.Trim( /* keep me */ []byte(s), " ")) // want `string\(bytes\.Trim\(\[\]byte\(s\), cutset\)\) copies`
}

func commentedBetween(s string) string {
	return string(bytes.TrimLeft([]byte(s) /* mid */, " ")) // want `string\(bytes\.TrimLeft\(\[\]byte\(s\), cutset\)\) copies`
}

func commentedAfter(s string) string {
	return string(bytes.TrimRight([]byte(s), " " /* tail */)) // want `string\(bytes\.TrimRight\(\[\]byte\(s\), cutset\)\) copies`
}
