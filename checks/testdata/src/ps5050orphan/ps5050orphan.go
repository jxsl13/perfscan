package ps5050orphan

import "slices"

// slices.Index on a byte slice is the ONLY slices use in this file — converting
// it would orphan the slices import, so the fix is withheld (reported advisory,
// golden unchanged).
func onlyUse(b []byte, c byte) int {
	return slices.Index(b, c) // want `slices\.Index over a byte slice`
}
