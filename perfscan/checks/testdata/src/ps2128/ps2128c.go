package ps2128

import . "strings"

// The file dot-imports strings: the package name strings is NOT bound, so
// the fix adds a separate plain strings import; both imports of the same
// path may coexist.
func dotImported(items []string) string {
	acc := "" // want `acc is grown by string concatenation on every loop iteration`
	for _, it := range items {
		acc += TrimSpace(it)
	}
	return acc
}
