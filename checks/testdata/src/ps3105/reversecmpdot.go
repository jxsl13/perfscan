package ps3105

import (
	. "cmp"
	"sort"
)

// cmp is DOT-imported: the descending fix cannot spell cmp.Compare by a
// usable package name (rewriting to a bare Compare(b, a) is too fragile),
// so the Reverse report stays advisory — golden equals source.
func reverseCmpDot(xs []int) {
	sort.Sort(sort.Reverse(sort.IntSlice(xs))) // want `sort\.Sort\(sort\.Reverse\(sort\.IntSlice\(\.\.\.\)\)\) sorts descending through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.SortFunc with cmp\.Compare\(b, a\) sorts the concrete \[\]int directly with the identical descending order`
	_ = Less(1, 2)
}
