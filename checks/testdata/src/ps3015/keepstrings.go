package ps3015

import (
	"fmt"
	"slices"
	"strings"
)

// The file keeps ANOTHER strings reference (the strings.ToUpper below), so
// the rewrite must NOT drop the strings import: only the comparator's own
// reference goes.
func sortedAndUpper(ys []string, s string) []string {
	out := slices.SortedFunc(slices.Values(ys), func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.Sorted collects and sorts the string elements in the identical byte-lexicographic order`
	fmt.Println(strings.ToUpper(s))
	return out
}
