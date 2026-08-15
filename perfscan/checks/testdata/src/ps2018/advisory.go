package ps2018

import "bytes"

// A NON-CONSTANT count keeps the report advisory: a runtime negative
// count panics in both forms, but with different message prefixes
// ("bytes: negative Repeat count" vs "strings: ..."), so the rewrite is
// not bit-identical for every possible count value.
func variableCount(s string, n int) string {
	return string(bytes.Repeat([]byte(s), n)) // want `string\(bytes\.Repeat\(\[\]byte\(s\), n\)\) copies`
}

// A NEGATIVE constant count is a guaranteed panic whose message the
// rewrite would change — advisory.
func negativeCount(s string) string {
	return string(bytes.Repeat([]byte(s), -1)) // want `string\(bytes\.Repeat\(\[\]byte\(s\), n\)\) copies`
}

// The name strings is owned by a local at the call site: the fix cannot
// qualify the callee there, so the report stays advisory and the file is
// left untouched (no import edits either — nothing is fixable).
func shadowedStrings(s string) string {
	strings := []string{s}
	_ = strings
	return string(bytes.Repeat([]byte(s), 3)) // want `string\(bytes\.Repeat\(\[\]byte\(s\), n\)\) copies`
}

// A comment inside the replaced punctuation would be silently destroyed
// by the rewrite — advisory.
func commentedInside(s string) string {
	return string(bytes.Repeat( /* keep me */ []byte(s), 2)) // want `string\(bytes\.Repeat\(\[\]byte\(s\), n\)\) copies`
}

func commentedBetween(s string) string {
	return string(bytes.Repeat([]byte(s) /* mid */, 2)) // want `string\(bytes\.Repeat\(\[\]byte\(s\), n\)\) copies`
}

func commentedAfter(s string) string {
	return string(bytes.Repeat([]byte(s), 2 /* tail */)) // want `string\(bytes\.Repeat\(\[\]byte\(s\), n\)\) copies`
}
