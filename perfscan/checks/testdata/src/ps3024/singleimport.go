package ps3024

import "sort"

// A single non-parenthesized import declaration whose only user is the
// rewritten call: the whole spec is swapped for "slices" in place.
func single(ss []string) bool {
	return sort.SliceIsSorted(ss, func(i, j int) bool { return ss[i] < ss[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}
