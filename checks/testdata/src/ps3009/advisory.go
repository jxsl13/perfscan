package ps3009

import (
	"fmt"
	"slices"
	"strings"
)

// A comment inside the span the rewrite would delete (after the slice
// argument) would be silently destroyed — advisory, no fix, golden
// identical. (This also keeps a strings reference alive, which is why the
// import survives in this file.)
func sortCommented(ys []string) {
	slices.SortFunc(ys, /* keep me */ func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.Sort sorts the string elements in the identical byte-lexicographic order with the comparison inlined`
	fmt.Println(ys)
}
