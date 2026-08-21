package ps3015

import (
	"slices"
	"strings"
)

// strings.Compare passed DIRECTLY as the comparator is the same anti-pattern
// minus the closure layer; it rewrites identically and the deletion drops
// this file's only strings reference, so the spec-in-group import is pruned.
func sortedFuncValue(ys []string) []string {
	return slices.SortedFunc(slices.Values(ys), strings.Compare) // want `slices\.Sorted collects and sorts the string elements in the identical byte-lexicographic order with the comparison inlined`
}
