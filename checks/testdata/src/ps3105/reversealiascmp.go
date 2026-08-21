package ps3105

import (
	c "cmp"
	s "slices"
	"sort"
)

// cmp and slices are already imported under ALIASES: the descending fix
// reuses both names and adds nothing; the orphaned sort import is dropped
// (this file's single Reverse site consumes all three sort references).
func reverseAliased(xs []int) {
	sort.Sort(sort.Reverse(sort.IntSlice(xs))) // want `sort\.Sort\(sort\.Reverse\(sort\.IntSlice\(\.\.\.\)\)\) sorts descending through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.SortFunc with cmp\.Compare\(b, a\) sorts the concrete \[\]int directly with the identical descending order`
	_ = c.Less(1, 2)
	s.Sort(xs)
}
