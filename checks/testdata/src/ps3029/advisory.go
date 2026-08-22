package ps3029

import (
	"cmp"
	"fmt"
	"slices"
)

// FLOAT elements: NaN compares neither '<' nor '>', so this comparator
// calls NaN equal to EVERYTHING — not even a consistent ordering — while
// slices.Sorted (via slices.Sort) orders NaNs first; and -0.0/+0.0 ties are
// equal-but-distinguishable, ties the STABLE sort contractually keeps in
// yield order while the unstable rewrite may arrange either way. Advisory,
// no fix.
func sortedFloats(fs []float64) []float64 {
	return slices.SortedStableFunc(slices.Values(fs), func(a, b float64) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Sorted collects and sorts the float64 elements ascending with a single inlined comparison and no stability cost \(no auto-fix: float elements — NaN compares neither < nor >, so this comparator calls NaN equal to everything while slices\.Sort orders NaNs first, and -0\.0/\+0\.0 ties are equal-but-distinguishable under an unstable sort\)`
}

// A TYPE-PARAMETER element may be instantiated with floats — the same NaN
// and distinguishable-tie hazards — advisory, no fix.
func sortedAny[T cmp.Ordered](xs []T) []T {
	return slices.SortedStableFunc(slices.Values(xs), func(a, b T) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Sorted collects and sorts the T elements ascending with a single inlined comparison and no stability cost \(no auto-fix: type-parameter element, instantiations may include floats — NaN makes this comparator inconsistent and slices\.Sort orders NaNs first\)`
}

// A comment inside the span the rewrite would delete — here the whole
// multi-line comparator body, this `want` comment included — would be
// silently destroyed: the report still fires, but stays advisory, no fix.
func sortedCommented(xs []int) []int {
	out := slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { // want `slices\.Sorted collects and sorts the int elements with the identical ascending order, a single inlined comparison and no stability cost`
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})
	fmt.Println(out)
	return out
}
