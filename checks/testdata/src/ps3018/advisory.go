package ps3018

import (
	"slices"
	"strings"
)

// A comment inside the span the rewrite would delete (after the seq
// argument) would be silently destroyed — advisory, no fix, golden
// identical. (This also keeps a strings reference alive, which is why the
// import survives in this file.)
func sortedCommented(ys []string) []string {
	return slices.SortedStableFunc(slices.Values(ys), /* keep me */ strings.Compare) // want `slices\.Sorted collects and sorts the string elements in the identical byte-lexicographic order`
}
