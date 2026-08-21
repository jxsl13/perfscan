package ps2124

// A single-element rewrite DROPS the separator; a package-qualified
// separator constant would lose this file's only net/http reference and
// orphan that import, so the fix is withheld (the strings import itself
// stays busy via ToLower).

import (
	"net/http"
	"strings"
)

var keepStrings = strings.ToLower("K")

func sepDropQualified(a string) string {
	return strings.Join([]string{a}, http.MethodGet) // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}

// A MULTI-element rewrite re-inserts the separator text between the
// elements, so the qualified reference survives and the fix is emitted.
func sepKeptQualified(a, b string) string {
	return strings.Join([]string{a, b}, http.MethodGet) // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}
