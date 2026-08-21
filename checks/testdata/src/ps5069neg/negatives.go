package ps5069neg

import (
	"bytes"
	"strings"
)

// A non-constant needle is out of scope: rewriting it to []byte(v) would
// trade the buffer copy for an operand copy of unknown size. Silent.
func nonConstNeedle(buf bytes.Buffer, sep string) bool {
	return strings.HasPrefix(buf.String(), sep)
}

// The empty string makes every predicate degenerate: silent.
func emptyNeedle(buf bytes.Buffer) bool {
	return strings.HasPrefix(buf.String(), "")
}

// strings.Builder.String() is a zero-copy view already — out of scope.
func builderReceiver(sb strings.Builder) bool {
	return strings.HasPrefix(sb.String(), "GET ")
}

// A strings function with no []byte-needle bytes twin in the recognized
// set (ToUpper) is out of scope: silent.
func unrecognizedFunc(buf bytes.Buffer) string {
	return strings.ToUpper(buf.String())
}

// strings.ContainsAny takes a string set, not a []byte needle: silent.
func containsAny(buf bytes.Buffer) bool {
	return strings.ContainsAny(buf.String(), "abc")
}

// The first argument is a plain string, not buf.String(): silent.
func plainString(s string) bool {
	return strings.HasPrefix(s, "GET ")
}
