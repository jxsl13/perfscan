package ps3013

import (
	"cmp"
	"fmt"
	"slices"
)

// FLOAT elements: NaN compares neither '<' nor '>', so this comparator
// calls NaN equal to EVERYTHING — not even a consistent ordering — while
// slices.Sort orders NaNs first; and -0.0/+0.0 ties are
// equal-but-distinguishable under an unstable sort. Advisory, no fix.
func sortFloats(fs []float64) {
	slices.SortFunc(fs, func(a, b float64) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Sort sorts the float64 elements ascending with a single inlined comparison \(no auto-fix: float elements — NaN compares neither < nor >, so this comparator calls NaN equal to everything while slices\.Sort orders NaNs first, and -0\.0/\+0\.0 ties are equal-but-distinguishable under an unstable sort\)`
	fmt.Println(fs)
}

// A TYPE-PARAMETER element may be instantiated with floats — the same NaN
// and distinguishable-tie hazards — advisory, no fix.
func sortAny[T cmp.Ordered](s []T) {
	slices.SortFunc(s, func(a, b T) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Sort sorts the T elements ascending with a single inlined comparison \(no auto-fix: type-parameter element, instantiations may include floats — NaN makes this comparator inconsistent and slices\.Sort orders NaNs first\)`
	fmt.Println(s)
}

// A comment inside the span the rewrite would delete — here the whole
// multi-line comparator body, this `want` comment included — would be
// silently destroyed: the report still fires, but stays advisory, no fix.
func sortCommented(xs []int) {
	slices.SortFunc(xs, func(a, b int) int { // want `slices\.Sort sorts the int elements with the identical ascending order and a single inlined comparison`
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})
	fmt.Println(xs)
}
