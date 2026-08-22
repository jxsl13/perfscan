package ps2017

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
	return string(bytes.Map(unicode.ToUpper, []byte(s))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}

// A comment inside the replaced punctuation would be silently destroyed
// by the rewrite — advisory.
func commentedBeforeF(s string) string {
	return string( /* keep me */ bytes.Map(unicode.ToUpper, []byte(s))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}

func commentedBetween(s string) string {
	return string(bytes.Map(unicode.ToLower /* mapping */, []byte(s))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}

func commentedAfter(s string) string {
	return string(bytes.Map(unicode.ToTitle, []byte(s) /* tail */)) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}

// The conversion is spelled through a type alias, not the literal
// []byte: deleting the alias reference could orphan the alias's import
// in the general case, so the fix is withheld by spelling.
type raw = []byte

func aliasSpelling(s string) string {
	return string(bytes.Map(unicode.ToUpper, raw(s))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}
