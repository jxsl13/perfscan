package ps3105

import (
	"sort"
)

// A MIXED file: one ascending site (TWO sort references) plus one Reverse
// site (THREE sort references) — the file's sort reference count is 5 =
// 2*1 + 3*1, exactly what the fixes consume, so the sort import is
// dropped; the fix adds both slices and cmp.
func mixedAscDesc(xs []int, ys []string) {
	sort.Sort(sort.IntSlice(xs))                  // want `sort\.Sort\(sort\.IntSlice\(\.\.\.\)\) sorts through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.Sort sorts the concrete \[\]int directly with the identical ascending order`
	sort.Sort(sort.Reverse(sort.StringSlice(ys))) // want `sort\.Sort\(sort\.Reverse\(sort\.StringSlice\(\.\.\.\)\)\) sorts descending through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.SortFunc with cmp\.Compare\(b, a\) sorts the concrete \[\]string directly with the identical descending order`
}
