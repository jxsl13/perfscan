package ps3016

import (
	c "cmp"
	sl "slices"
)

// Both packages are resolved by IMPORT PATH, not spelling: an aliased slices
// keeps its alias in the rewrite, and the aliased cmp spec is dropped when
// the rewrite deletes its last reference.
func sortedAliased(xs []int) []int {
	return sl.SortedStableFunc(sl.Values(xs), func(a, b int) int { return c.Compare(a, b) }) // want `slices\.SortedStableFunc with a bare cmp\.Compare comparator pays an indirect comparator call per comparison plus the stable sort's merge overhead; slices\.Sorted collects and sorts the int elements`
}
