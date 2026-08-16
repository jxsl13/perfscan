package ps3037add

import "sort"

// A non-search sort use keeps sort alive after the rewrite, so the fix fires and
// must ADD the slices import.
var _ = sort.IntsAreSorted

func addSlices(a []int, x int) int {
	i := sort.SearchInts(a, x) // want `sort\.Search\* runs a binary search through a per-probe closure; slices\.BinarySearch`
	return i
}
