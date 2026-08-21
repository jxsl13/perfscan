package ps5069

import (
	"bytes"
	"strings"
)

// A residual strings use so the rewrite never orphans the strings import
// (the fix is withheld file-wide otherwise, mirroring PS5050).
func keep(s string) string { return strings.ToUpper(s) }

func hasPrefix(buf bytes.Buffer) bool {
	return strings.HasPrefix(buf.String(), "GET ") // want `strings\.HasPrefix over buf\.String\(\) copies the whole bytes\.Buffer just to test it against a constant`
}

func hasSuffix(buf bytes.Buffer) bool {
	return strings.HasSuffix(buf.String(), "\r\n") // want `strings\.HasSuffix over buf\.String\(\) copies the whole bytes\.Buffer just to test it against a constant`
}

func contains(buf bytes.Buffer) bool {
	return strings.Contains(buf.String(), "\n\n") // want `strings\.Contains over buf\.String\(\) copies the whole bytes\.Buffer just to test it against a constant`
}

func equalFold(buf bytes.Buffer) bool {
	return strings.EqualFold(buf.String(), "OK") // want `strings\.EqualFold over buf\.String\(\) copies the whole bytes\.Buffer just to test it against a constant`
}

func index(buf bytes.Buffer) int {
	return strings.Index(buf.String(), "sep") // want `strings\.Index over buf\.String\(\) copies the whole bytes\.Buffer just to test it against a constant`
}

func lastIndex(buf bytes.Buffer) int {
	return strings.LastIndex(buf.String(), "sep") // want `strings\.LastIndex over buf\.String\(\) copies the whole bytes\.Buffer just to test it against a constant`
}

func count(buf bytes.Buffer) int {
	return strings.Count(buf.String(), "x") // want `strings\.Count over buf\.String\(\) copies the whole bytes\.Buffer just to test it against a constant`
}

// A pointer receiver taken by &buf is provably non-nil: fixed.
func pointerAddr(buf bytes.Buffer) bool {
	return strings.HasPrefix((&buf).String(), "GET ") // want `strings\.HasPrefix over buf\.String\(\) copies the whole bytes\.Buffer just to test it against a constant`
}

// new(bytes.Buffer) is provably non-nil: fixed.
func pointerNew() bool {
	return strings.HasPrefix(new(bytes.Buffer).String(), "GET ") // want `strings\.HasPrefix over buf\.String\(\) copies the whole bytes\.Buffer just to test it against a constant`
}

// A named string constant carries over verbatim into the []byte conversion.
const prefix = "POST "

func namedConst(buf bytes.Buffer) bool {
	return strings.HasPrefix(buf.String(), prefix) // want `strings\.HasPrefix over buf\.String\(\) copies the whole bytes\.Buffer just to test it against a constant`
}
