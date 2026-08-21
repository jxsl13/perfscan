package ps3009

import (
	"fmt"
	"slices"
	"strings"
)

// Explicit instantiation brackets SURVIVE the rewrite: slices.Sort takes
// the same two type parameters as SortFunc/SortStableFunc and string
// satisfies cmp.Ordered, so only the selector name changes and the
// comparator goes.
func sortInstantiated(ys []string) {
	slices.SortFunc[[]string, string](ys, func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.Sort sorts the string elements in the identical byte-lexicographic order with the comparison inlined`
	slices.SortStableFunc[[]string](ys, strings.Compare)                                          // want `slices\.Sort sorts the string elements in the identical byte-lexicographic order, the comparison inlined and no stability cost`
	fmt.Println(ys)
}
