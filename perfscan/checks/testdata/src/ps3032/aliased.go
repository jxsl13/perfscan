package ps3032

import (
	sl "slices"
	"sort"
)

// slices is imported under an ALIAS: the fix spells the call with the
// alias and deletes the now-orphaned sort spec.
func aliasedSlices(xs []int) bool {
	ok := sort.IsSorted(sort.IntSlice(xs)) // want `sort\.IsSorted\(sort\.IntSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
	sl.Reverse(xs)
	return ok
}
