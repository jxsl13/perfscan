package ps3018

import (
	"maps"
	"slices"
	"strings"
)

// The bare strings.Compare comparator is slices.Sorted spelled the slow,
// stable way; the rewrites delete this file's ONLY strings references, so the
// fix drops the orphaned strings import. The seq expression is kept verbatim.

// A func literal wrapping strings.Compare.
func sortedKeys(m map[string]int) []string {
	return slices.SortedStableFunc(maps.Keys(m), func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.SortedStableFunc with a bare strings\.Compare\(a, b\) comparator pays an indirect comparator call plus a strings\.Compare call per comparison, and the stable sort's merge overhead on top; slices\.Sorted collects and sorts the string elements in the identical byte-lexicographic order with the comparison inlined and no stability cost`
}

// The two-field parameter spelling func(a T, b T) matches like func(a, b T),
// and parameter names are matched by object identity, not spelling.
func sortedVals(ys []string) []string {
	return slices.SortedStableFunc(slices.Values(ys), func(x string, y string) int { return strings.Compare(x, y) }) // want `slices\.Sorted collects and sorts the string elements in the identical byte-lexicographic order with the comparison inlined and no stability cost`
}
