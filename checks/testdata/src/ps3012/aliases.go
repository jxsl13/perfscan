package ps3012

import (
	c "cmp"
	sl "slices"
)

// Both packages are resolved by IMPORT PATH, not spelling: an aliased slices
// keeps its alias in the rewrite, and the aliased cmp spec is dropped when
// the rewrite deletes its last reference.
func sortedAliased(xs []int) []int {
	return sl.SortedFunc(sl.Values(xs), func(a, b int) int { return c.Compare(a, b) }) // want `slices\.SortedFunc with a bare cmp\.Compare comparator pays an indirect comparator call per comparison; slices\.Sorted collects and sorts the int elements`
}
