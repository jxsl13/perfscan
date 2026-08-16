package ps5050add

import "slices"

// A non-byte slices use keeps slices alive after the byte-slice call is
// converted, so the fix fires and must ADD the bytes import.
var keep = slices.Contains([]int{1}, 2)

func addBytes(b []byte, c byte) int {
	return slices.Index(b, c) // want `slices\.Index over a byte slice`
}
