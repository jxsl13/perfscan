package ps3010

import (
	"cmp"
	"slices"
)

// Swapped operands ask whether the slice is DESCENDING — never matched.
func swapped(xs []int) bool {
	return slices.IsSortedFunc(xs, func(a, b int) int { return cmp.Compare(b, a) })
}

// A custom comparator, not cmp.Compare.
func custom(xs []int) bool {
	return slices.IsSortedFunc(xs, func(a, b int) int {
		if a > b {
			return 1
		}
		return -1
	})
}

// A comparator with extra computation fails the exact match.
func extra(xs []int) bool {
	return slices.IsSortedFunc(xs, func(a, b int) int {
		r := cmp.Compare(a, b)
		return r
	})
}

// A field selector inside the comparator fails the exact match.
type pair struct{ k int }

func fields(ps []pair) bool {
	return slices.IsSortedFunc(ps, func(a, b pair) int { return cmp.Compare(a.k, b.k) })
}

// Already the direct call.
func direct(xs []int) bool {
	return slices.IsSorted(xs)
}

// A comment inside the deleted span would be destroyed: reported, but the fix
// is withheld (advisory) — the golden file keeps this call unchanged.
func commented(xs []int) bool {
	return slices.IsSortedFunc(xs, cmp.Compare /* keep me */) // want `slices\.IsSortedFunc with a bare cmp\.Compare comparator`
}
