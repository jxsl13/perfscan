package ps3026

import (
	sl "slices"
)

// The package is resolved by IMPORT PATH, not spelling: an aliased slices
// keeps its alias in the rewrite. The comparator references nothing but its
// own parameters and literals, so no import is ever orphaned.
func sortedAliased(xs []int) []int {
	return sl.SortedFunc(sl.Values(xs), func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.SortedFunc with a hand-rolled three-way comparator \(a<b/a>b/-1/1/0\) pays an indirect comparator call plus up to two relational comparisons per comparison; slices\.Sorted collects and sorts the int elements with the identical ascending order and a single inlined comparison`
}
