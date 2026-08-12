package ps3105

import (
	"fmt"
	"sort"
)

// Both adapter spellings are rewritten; these calls are the file's ONLY
// sort references (two per call: sort.Sort plus the conversion), so the
// fix also swaps the orphaned sort import for slices.
func sortBoth(xs []int, ys []string) {
	sort.Sort(sort.IntSlice(xs))    // want `sort\.Sort\(sort\.IntSlice\(\.\.\.\)\) sorts through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.Sort sorts the concrete \[\]int directly with the identical ascending order`
	sort.Sort(sort.StringSlice(ys)) // want `sort\.Sort\(sort\.StringSlice\(\.\.\.\)\) sorts through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.Sort sorts the concrete \[\]string directly with the identical ascending order`
	fmt.Println(xs, ys)
}

// The operand expression is kept verbatim, however it is spelled.
func sortField(w struct{ ids []int }) {
	sort.Sort(sort.IntSlice(w.ids)) // want `sort\.Sort\(sort\.IntSlice\(\.\.\.\)\) sorts through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.Sort sorts the concrete \[\]int directly with the identical ascending order`
	fmt.Println(w.ids)
}
