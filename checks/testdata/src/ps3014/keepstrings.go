package ps3014

import (
	"slices"
	"strings"
)

// The file keeps ANOTHER strings reference (the strings.ToUpper below), so
// the rewrite must NOT drop the strings import: only the comparator's own
// reference goes.
func sortedAndUpper(ys []string, t string) bool {
	ok := slices.IsSortedFunc(ys, func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.IsSortedFunc with a bare strings\.Compare comparator`
	_ = strings.ToUpper(t)
	return ok
}
