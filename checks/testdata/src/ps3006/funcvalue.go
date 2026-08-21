package ps3006

import (
	"cmp"
	"fmt"
	"slices"
)

// cmp.Compare passed DIRECTLY as the comparator is the same anti-pattern
// minus the closure layer; it rewrites identically and its deletion drops
// this file's only cmp reference, so the spec-in-group import is pruned.
func sortFuncValue(xs []int, ys []string) {
	slices.SortStableFunc(xs, cmp.Compare) // want `slices\.Sort sorts the int elements with the identical ascending order, the comparison inlined and no stability cost`
	slices.SortStableFunc(ys, cmp.Compare) // want `slices\.Sort sorts the string elements with the identical ascending order, the comparison inlined and no stability cost`
	fmt.Println(xs, ys)
}
