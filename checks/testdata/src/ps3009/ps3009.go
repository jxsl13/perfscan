package ps3009

import (
	"fmt"
	"slices"
	"strings"
)

// The bare-strings.Compare comparator is slices.Sort spelled the slow way;
// the rewrites below delete this file's ONLY strings references, so the fix
// also drops the orphaned strings import. The slice expression is kept
// verbatim.
func sortStrings(ys []string) {
	slices.SortFunc(ys, func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.SortFunc with a bare strings\.Compare\(a, b\) comparator pays an indirect comparator call plus a strings\.Compare call per comparison; slices\.Sort sorts the string elements in the identical byte-lexicographic order with the comparison inlined`
	fmt.Println(ys)
}

// The STABLE spelling drops the symMerge overhead too: byte-equal ties are
// interchangeable, so stability buys nothing observable on strings.
func sortStable(ys []string) {
	slices.SortStableFunc(ys, func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.SortStableFunc with a bare strings\.Compare\(a, b\) comparator pays an indirect comparator call plus a strings\.Compare call per comparison and the stable sort's merge overhead; slices\.Sort sorts the string elements in the identical byte-lexicographic order, the comparison inlined and no stability cost`
	fmt.Println(ys)
}

// Parameter names are matched by object identity, not spelling, and the
// operand expression is kept verbatim, however it is spelled.
func sortField(w struct{ names []string }) {
	slices.SortFunc(w.names, func(x, y string) int { return strings.Compare(x, y) }) // want `slices\.Sort sorts the string elements in the identical byte-lexicographic order with the comparison inlined`
	fmt.Println(w.names)
}

// The two-field parameter spelling func(a T, b T) matches like func(a, b T),
// and a deferred call rewrites the same way (both spellings evaluate the
// slice expression at defer time and sort at function exit).
func sortDeferred(ys []string) {
	defer slices.SortFunc(ys, func(a string, b string) int { return strings.Compare(a, b) }) // want `slices\.Sort sorts the string elements in the identical byte-lexicographic order with the comparison inlined`
	fmt.Println(ys)
}
