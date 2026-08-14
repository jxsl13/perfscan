package ps3006

import (
	"cmp"
	"fmt"
	"slices"
)

// FLOAT elements: the ORDER is identical (cmp.Compare and slices.Sort
// both put NaNs first), but -0.0/+0.0 and distinct NaN payloads are
// equal-but-distinguishable ties whose original order SortStableFunc
// contractually preserves and the unstable rewrite may flip — advisory,
// no fix.
func sortFloats(fs []float64) {
	slices.SortStableFunc(fs, func(a, b float64) int { return cmp.Compare(a, b) }) // want `slices\.Sort sorts the float64 elements with the identical ascending order, the comparison inlined and no stability cost \(no auto-fix: float ties -0\.0/\+0\.0 and NaN payloads are equal-but-distinguishable under an unstable sort\)`
	fmt.Println(fs)
}

// A TYPE-PARAMETER element may be instantiated with floats — same
// distinguishable-tie hazard — advisory, no fix.
func sortAny[T cmp.Ordered](s []T) {
	slices.SortStableFunc(s, func(a, b T) int { return cmp.Compare(a, b) }) // want `slices\.Sort sorts the T elements with the identical ascending order, the comparison inlined and no stability cost \(no auto-fix: type-parameter element, instantiations may include floats whose ties are equal-but-distinguishable\)`
	fmt.Println(s)
}

// A comment inside the span the rewrite would delete (after the slice
// argument) would be silently destroyed — advisory, no fix.
func sortCommented(xs []int) {
	slices.SortStableFunc(xs, /* keep me */ func(a, b int) int { return cmp.Compare(a, b) }) // want `slices\.Sort sorts the int elements with the identical ascending order, the comparison inlined and no stability cost`
	fmt.Println(xs)
}
