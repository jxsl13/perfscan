package ps5011

import "bytes"

// The name strings is owned by a local at the call site: the fix cannot
// qualify the callee there, so the report stays advisory and the file is
// left untouched (no import edits either — nothing is fixable).
func shadowedStrings(s string) string {
	strings := []string{s}
	_ = strings
	return string(bytes.ReplaceAll([]byte(s), []byte(" "), []byte("_"))) // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`
}

// A comment inside the replaced punctuation would be silently destroyed
// by the rewrite — advisory.
func commentedInside(s string) string {
	return string(bytes.ReplaceAll( /* keep me */ []byte(s), []byte("a"), []byte("b"))) // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`
}

func commentedBetween(s string) string {
	return string(bytes.ReplaceAll([]byte(s) /* mid */, []byte("a"), []byte("b"))) // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`
}

func commentedBeforeNew(s string) string {
	return string(bytes.ReplaceAll([]byte(s), []byte("a") /* pre-new */, []byte("b"))) // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`
}

func commentedBeforeN(s string, n int) string {
	return string(bytes.Replace([]byte(s), []byte("a"), []byte("b") /* pre-n */, n)) // want `string\(bytes\.Replace\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\), n\)\) copies`
}

func commentedAfter(s string, n int) string {
	return string(bytes.Replace([]byte(s), []byte("a"), []byte("b"), n /* tail */)) // want `string\(bytes\.Replace\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\), n\)\) copies`
}
