package ps3107

import (
	"cmp"
	"fmt"
	"slices"
)

// Explicit instantiation brackets SURVIVE the rewrite: slices.Sort takes
// the same two type parameters as slices.SortFunc, so only the selector
// name changes and the comparator goes.
func sortInstantiated(xs []int) {
	slices.SortFunc[[]int, int](xs, func(a, b int) int { return cmp.Compare(a, b) }) // want `slices\.Sort sorts the int elements with the identical ascending order and the comparison inlined`
	slices.SortFunc[[]int](xs, cmp.Compare)                                          // want `slices\.Sort sorts the int elements with the identical ascending order and the comparison inlined`
	fmt.Println(xs)
}
