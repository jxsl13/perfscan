package ps3016

import (
	"cmp"
	"fmt"
	"slices"
)

// The file keeps ANOTHER cmp reference (the cmp.Or below), so the rewrite
// must NOT drop the cmp import: only the comparator's own reference goes.
func sortedAndOr(xs []int, s string) []int {
	out := slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { return cmp.Compare(a, b) }) // want `slices\.Sorted collects and sorts the int elements`
	fmt.Println(cmp.Or(s, "fallback"))
	return out
}
