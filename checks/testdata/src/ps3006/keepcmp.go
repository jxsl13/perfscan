package ps3006

import (
	"cmp"
	"fmt"
	"slices"
)

// The file keeps ANOTHER cmp reference (the cmp.Or below), so the rewrite
// must NOT drop the cmp import: only the comparator's own reference goes.
func sortAndOr(xs []int, s string) {
	slices.SortStableFunc(xs, func(a, b int) int { return cmp.Compare(a, b) }) // want `slices\.Sort sorts the int elements with the identical ascending order, the comparison inlined and no stability cost`
	fmt.Println(cmp.Or(s, "fallback"))
}
