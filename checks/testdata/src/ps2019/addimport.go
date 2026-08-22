package ps2019

import (
	"strings"
)

// bytes is NOT imported here: the fixable site's fix also inserts the
// bytes import at its sorted position; the strings.ToLower reference on
// a real string keeps strings alive, so only the addition is needed.
func addImport(s string, b, sub []byte) bool {
	if strings.ToLower(s) == "" {
		return false
	}
	return strings.Contains(string(b), string(sub)) // want `strings\.Contains\(string\(b\), string\(sub\)\) allocates two throwaway string copies just to scan them; bytes\.Contains\(b, sub\) runs the same scan on the bytes directly with zero allocations`
}
