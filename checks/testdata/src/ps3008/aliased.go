package ps3008

import (
	sl "slices"
	"sort"
)

// slices is imported under an ALIAS: the fix spells the call with the
// alias and deletes the now-orphaned sort spec.
func aliasedSlices(xs []int) bool {
	ok := sort.IntsAreSorted(xs) // want `sort\.IntsAreSorted is the legacy spelling of slices\.IsSorted \(an interface-dispatch scan on go1\.21, a one-line wrapper since go1\.22\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
	sl.Reverse(xs)
	return ok
}
