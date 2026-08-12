package ps3105

import "sort"

// A single non-parenthesized import declaration whose only users are the
// rewritten call's two references: the whole spec is swapped for "slices"
// in place.
func single(ys []string) {
	sort.Sort(sort.StringSlice(ys)) // want `sort\.Sort\(sort\.StringSlice\(\.\.\.\)\) sorts through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.Sort sorts the concrete \[\]string directly with the identical ascending order`
}
