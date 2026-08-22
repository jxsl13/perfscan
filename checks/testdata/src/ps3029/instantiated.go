package ps3029

import (
	"slices"
)

// An explicit instantiation bracket SURVIVES the rewrite: slices.Sorted
// takes the same single element type parameter as slices.SortedStableFunc,
// and every fixable element kind satisfies its cmp.Ordered constraint, so
// only the selector name changes and the comparator goes.
func sortedInstantiated(xs []int) []int {
	return slices.SortedStableFunc[int](slices.Values(xs), func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Sorted collects and sorts the int elements with the identical ascending order, a single inlined comparison and no stability cost`
}
