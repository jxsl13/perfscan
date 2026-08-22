package ps3017

import (
	"fmt"
	"slices"
)

// Explicit instantiation brackets SURVIVE the rewrite: slices.Sort takes
// the same two type parameters as slices.SortStableFunc (and every fixable
// element type satisfies its cmp.Ordered constraint), so only the selector
// name changes and the comparator goes.
func sortInstantiated(xs []int) {
	slices.SortStableFunc[[]int, int](xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Sort sorts the int elements with the identical ascending order, a single inlined comparison and no stability cost`
	slices.SortStableFunc[[]int](xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 })      // want `slices\.Sort sorts the int elements with the identical ascending order, a single inlined comparison and no stability cost`
	fmt.Println(xs)
}
