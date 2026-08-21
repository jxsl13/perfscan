package ps3036

import (
	"slices"
	"strings"
)

// The file keeps ANOTHER strings reference (the strings.ToUpper below), so
// the rewrite must NOT drop the strings import: only the comparator's own
// reference goes.
func compareAndUpper(a, b []string) int {
	d := slices.CompareFunc(a, b, func(x, y string) int { return strings.Compare(x, y) }) // want `slices\.CompareFunc with a bare strings\.Compare comparator`
	_ = strings.ToUpper("keep")
	return d
}
