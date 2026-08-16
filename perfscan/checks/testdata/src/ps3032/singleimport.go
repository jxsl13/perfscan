package ps3032

import "sort"

// A single non-parenthesized import declaration whose only users are the
// rewritten call's two references: the whole spec is swapped for "slices"
// in place.
func single(ys []string) bool {
	return sort.IsSorted(sort.StringSlice(ys)) // want `sort\.IsSorted\(sort\.StringSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]string directly with the identical boolean result`
}
