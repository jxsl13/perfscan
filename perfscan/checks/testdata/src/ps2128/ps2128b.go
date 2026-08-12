package ps2128

import "strings"

// strings is already imported and used in this file: the fix must reuse
// that import, not add a duplicate.
func joinImported(items []string) string {
	acc := "" // want `acc is grown by string concatenation on every loop iteration`
	for _, it := range items {
		acc += strings.TrimSpace(it)
	}
	return acc
}
