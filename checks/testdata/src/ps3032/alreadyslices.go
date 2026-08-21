package ps3032

import (
	"slices"
	"sort"
)

// slices is ALREADY imported: the fix must not duplicate the import, and
// the now-orphaned sort spec (both of its references belong to the one
// rewritten call) is deleted from the group.
func alreadySlices(xs []int) bool {
	ok := sort.IsSorted(sort.IntSlice(xs)) // want `sort\.IsSorted\(sort\.IntSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
	slices.Reverse(xs)
	return ok
}
