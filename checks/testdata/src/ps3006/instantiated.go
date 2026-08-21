package ps3006

import (
	"cmp"
	"fmt"
	"slices"
)

// Explicit instantiation brackets SURVIVE the rewrite: slices.Sort takes
// the same two type parameters as slices.SortStableFunc, so only the
// selector name changes and the comparator goes.
func sortInstantiated(xs []int) {
	slices.SortStableFunc[[]int, int](xs, func(a, b int) int { return cmp.Compare(a, b) }) // want `slices\.Sort sorts the int elements with the identical ascending order, the comparison inlined and no stability cost`
	slices.SortStableFunc[[]int](xs, cmp.Compare)                                          // want `slices\.Sort sorts the int elements with the identical ascending order, the comparison inlined and no stability cost`
	fmt.Println(xs)
}
