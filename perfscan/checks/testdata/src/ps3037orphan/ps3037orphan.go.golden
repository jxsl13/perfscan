package ps3037orphan

import "sort"

// sort.SearchInts is the file's only sort reference: fixing would orphan the
// import, so the report stays advisory.
func onlyRef(a []int, x int) int {
	i := sort.SearchInts(a, x) // want `sort\.Search\* runs a binary search through a per-probe closure; slices\.BinarySearch`
	return i
}
