package ps3011

import (
	"slices"
	"strings"
)

// A comment inside the span the rewrite would delete (after the target
// argument) would be silently destroyed — advisory, no fix, golden identical.
// (This also keeps a strings reference alive, which is why the import survives
// in this file.)
func searchCommented(ys []string, t string) (int, bool) {
	return slices.BinarySearchFunc(ys, t /* keep me */, func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.BinarySearchFunc with a bare strings\.Compare comparator .* \(no auto-fix:`
}
