package ps3024

import (
	sl "slices"
	"sort"
)

// slices is imported under an ALIAS: the fix spells the call with the
// alias and deletes the now-orphaned sort spec.
func aliasedSlices(xs []int) bool {
	ok := sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] < xs[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
	sl.Reverse(xs)
	return ok
}
