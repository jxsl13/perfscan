package ps3019

import (
	"slices"
)

// Explicit instantiation brackets SURVIVE the rewrite: slices.IsSorted
// takes the same two type parameters as slices.IsSortedFunc (and every
// fixable element type satisfies its cmp.Ordered constraint), so only the
// selector name changes and the comparator goes.
func checkInstantiated(xs []int) {
	_ = slices.IsSortedFunc[[]int, int](xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.IsSorted answers the identical bool over the int elements with a single inlined comparison`
	_ = slices.IsSortedFunc[[]int](xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 })      // want `slices\.IsSorted answers the identical bool over the int elements with a single inlined comparison`
}
