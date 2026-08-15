package ps3012

import (
	"cmp"
	"slices"
)

// An explicit instantiation bracket SURVIVES the rewrite: slices.Sorted takes
// the same single element type parameter as slices.SortedFunc, and the fixable
// element kinds satisfy its cmp.Ordered constraint, so only the selector name
// changes and the comparator goes.
func sortedInstantiated(xs []int) []int {
	return slices.SortedFunc[int](slices.Values(xs), cmp.Compare) // want `slices\.Sorted collects and sorts the int elements`
}
