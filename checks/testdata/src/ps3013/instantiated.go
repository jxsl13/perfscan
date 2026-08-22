package ps3013

import (
	"fmt"
	"slices"
)

// Explicit instantiation brackets SURVIVE the rewrite: slices.Sort takes
// the same two type parameters as slices.SortFunc (and every fixable
// element type satisfies its cmp.Ordered constraint), so only the selector
// name changes and the comparator goes.
func sortInstantiated(xs []int) {
	slices.SortFunc[[]int, int](xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Sort sorts the int elements with the identical ascending order and a single inlined comparison`
	slices.SortFunc[[]int](xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 })      // want `slices\.Sort sorts the int elements with the identical ascending order and a single inlined comparison`
	fmt.Println(xs)
}
