package ps3018

import (
	"slices"
	"strings"
)

// An explicit instantiation bracket SURVIVES the rewrite: slices.Sorted
// takes the same single element type parameter as slices.SortedStableFunc,
// and string satisfies its cmp.Ordered constraint, so only the selector name
// changes and the comparator goes.
func sortedInstantiated(ys []string) []string {
	return slices.SortedStableFunc[string](slices.Values(ys), strings.Compare) // want `slices\.Sorted collects and sorts the string elements in the identical byte-lexicographic order`
}
