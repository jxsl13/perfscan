package ps3105

import (
	"fmt"
	"sort"
)

// The descending sort.Reverse idiom is rewritten to slices.SortFunc with a
// cmp.Compare(b, a) comparator — the identical descending predicate under
// the same unstable pdqsort. Each fixable Reverse call consumes THREE sort
// references (sort.Sort + sort.Reverse + the conversion); these calls are
// the file's ONLY sort references, so the fix also swaps the orphaned sort
// import for slices AND adds the missing cmp import.
func reverseBoth(xs []int, ys []string) {
	sort.Sort(sort.Reverse(sort.IntSlice(xs)))    // want `sort\.Sort\(sort\.Reverse\(sort\.IntSlice\(\.\.\.\)\)\) sorts descending through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.SortFunc with cmp\.Compare\(b, a\) sorts the concrete \[\]int directly with the identical descending order`
	sort.Sort(sort.Reverse(sort.StringSlice(ys))) // want `sort\.Sort\(sort\.Reverse\(sort\.StringSlice\(\.\.\.\)\)\) sorts descending through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.SortFunc with cmp\.Compare\(b, a\) sorts the concrete \[\]string directly with the identical descending order`
	fmt.Println(xs, ys)
}

// sort.Stable(sort.Reverse(...)) rewrites to the same UNSTABLE
// slices.SortFunc: for []int stability is unobservable (equal elements are
// bitwise-identical), so the descending arrangement is unique either way.
func reverseStable(xs []int) {
	sort.Stable(sort.Reverse(sort.IntSlice(xs))) // want `sort\.Stable\(sort\.Reverse\(sort\.IntSlice\(\.\.\.\)\)\) sorts descending through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.SortFunc with cmp\.Compare\(b, a\) sorts the concrete \[\]int directly with the identical descending order`
}

// The operand expression is kept verbatim, however it is spelled.
func reverseField(w struct{ ids []int }) {
	sort.Sort(sort.Reverse(sort.IntSlice(w.ids))) // want `sort\.Sort\(sort\.Reverse\(sort\.IntSlice\(\.\.\.\)\)\) sorts descending through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.SortFunc with cmp\.Compare\(b, a\) sorts the concrete \[\]int directly with the identical descending order`
	fmt.Println(w.ids)
}
