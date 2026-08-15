package ps3014

import (
	"slices"
	"strings"
)

// A comment inside the span the rewrite would delete (after the slice
// argument) would be silently destroyed — advisory, no fix, golden identical.
// (This also keeps a strings reference alive, which is why the import
// survives in this file.)
func sortedCommented(ys []string) bool {
	return slices.IsSortedFunc(ys /* keep me */, func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.IsSortedFunc with a bare strings\.Compare comparator .* \(no auto-fix:`
}
