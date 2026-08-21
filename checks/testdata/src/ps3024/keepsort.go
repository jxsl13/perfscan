package ps3024

import (
	"sort"
)

// The file keeps OTHER sort references (sort.Ints, and an advisory
// descending SliceIsSorted), so the sort import stays; the fix only
// inserts the slices import.
func keepSort(xs []int) bool {
	ok := sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] < xs[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`

	// Advisory (descending) — keeps a surviving sort reference.
	ok = ok && sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] > xs[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`

	// NEVER flagged by PS3024: a different sort function entirely.
	sort.Ints(xs)
	return ok
}
