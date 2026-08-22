package ps3009

import (
	"fmt"
	"slices"
	"strings"
)

// The file keeps ANOTHER strings reference (the strings.ToUpper below), so
// the rewrite must NOT drop the strings import: only the comparator's own
// reference goes.
func sortAndUpper(ys []string, s string) {
	slices.SortFunc(ys, func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.Sort sorts the string elements in the identical byte-lexicographic order with the comparison inlined`
	fmt.Println(strings.ToUpper(s))
}
