package ps3019

import (
	"cmp"
	"fmt"
	"slices"
)

// FLOAT elements: a NaN compares neither '<' nor '>', so this comparator
// calls NaN equal to EVERYTHING and the scan sails past it, while
// slices.IsSorted orders NaNs first — IsSortedFunc([1.0, NaN], chain) is
// true, slices.IsSorted([1.0, NaN]) is false. The bool itself can differ:
// advisory, no fix.
func checkFloats(fs []float64) bool {
	return slices.IsSortedFunc(fs, func(a, b float64) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.IsSorted scans the float64 elements with a single inlined comparison \(no auto-fix: float elements — a NaN compares neither < nor >, so this comparator calls NaN equal to everything and the scan can answer true where slices\.IsSorted, which orders NaNs first, answers false\)`
}

// A TYPE-PARAMETER element may be instantiated with floats — the same NaN
// hazard — advisory, no fix.
func checkAny[T cmp.Ordered](s []T) bool {
	return slices.IsSortedFunc(s, func(a, b T) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.IsSorted scans the T elements with a single inlined comparison \(no auto-fix: type-parameter element, instantiations may include floats — a NaN makes this comparator call NaN equal to everything while slices\.IsSorted orders NaNs first, so the bool can differ\)`
}

// A comment inside the span the rewrite would delete — here the whole
// multi-line comparator body, this `want` comment included — would be
// silently destroyed: the report still fires, but stays advisory, no fix.
func checkCommented(xs []int) bool {
	ok := slices.IsSortedFunc(xs, func(a, b int) int { // want `slices\.IsSorted answers the identical bool over the int elements with a single inlined comparison`
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})
	fmt.Println(ok)
	return ok
}
