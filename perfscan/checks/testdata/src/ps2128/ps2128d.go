package ps2128

import stx "strings"

// The strings import is RENAMED: the fix must add a plain strings import
// alongside it rather than referencing str.
func renamedImport(items []string) string {
	acc := "" // want `acc is grown by string concatenation on every loop iteration`
	for _, it := range items {
		acc += stx.ToUpper(it)
	}
	return acc
}
