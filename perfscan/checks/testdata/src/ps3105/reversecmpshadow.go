package ps3105

import (
	"sort"
)

// A local named cmp owns the name at the call site: the descending fix
// cannot reference the cmp package there, so the report stays advisory —
// golden equals source. (The ascending fix never needs cmp, so this guard
// is specific to the Reverse form.)
func reverseCmpShadowed(xs []int) {
	cmp := 1
	_ = cmp
	sort.Sort(sort.Reverse(sort.IntSlice(xs))) // want `sort\.Sort\(sort\.Reverse\(sort\.IntSlice\(\.\.\.\)\)\) sorts descending through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.SortFunc with cmp\.Compare\(b, a\) sorts the concrete \[\]int directly with the identical descending order`
}
