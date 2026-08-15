package ps3024cgo

// A cgo file's import block must never be edited, and this fix would need
// to ADD the slices import (and drop the orphaned sort one), so PS3024
// offers no fix here at all: the report stays advisory and the golden is
// identical.

import "C"

import "sort"

func cgoSorted(xs []int) bool {
	return sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] < xs[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}
