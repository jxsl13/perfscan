package ps3028

import (
	"cmp"
	"fmt"
	"slices"
)

// FLOAT elements: NaN compares neither '<' nor '>', so this comparator
// answers 0 for a NaN against ANYTHING — BinarySearchFunc stops at the first
// NaN probe and reports found=true there, while slices.BinarySearch orders
// NaN first and reports found only for a real match. Advisory, no fix.
func searchFloats(fs []float64, target float64) (int, bool) {
	return slices.BinarySearchFunc(fs, target, func(a, b float64) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.BinarySearch searches the float64 elements with the comparison inlined \(no auto-fix: float elements — NaN compares neither < nor >, so this comparator answers 0 for NaN against anything and reports found at a NaN probe while slices\.BinarySearch orders NaN first\)`
}

// A TYPE-PARAMETER element may be instantiated with floats — the same NaN
// hazard — advisory, no fix.
func searchAny[T cmp.Ordered](s []T, target T) (int, bool) {
	return slices.BinarySearchFunc(s, target, func(a, b T) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.BinarySearch searches the T elements with the comparison inlined \(no auto-fix: type-parameter element, instantiations may include floats — this comparator answers 0 for NaN against anything, flipping the found bit, while slices\.BinarySearch orders NaN first\)`
}

// An explicit instantiation: BinarySearchFunc's THREE type arguments do not
// transfer to BinarySearch's two — reported, but advisory (not auto-fixed).
func searchInstantiated(xs []int, target int) (int, bool) {
	return slices.BinarySearchFunc[[]int, int, int](xs, target, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.BinarySearch searches the int elements with the identical \(index, found\) result and the comparison inlined \(no auto-fix: an explicit instantiation's three type arguments do not transfer to BinarySearch's two, or a comment in the deleted span\)`
}

// A comment inside the span the rewrite would delete — here the whole
// multi-line comparator body, this `want` comment included — would be
// silently destroyed: the report still fires, but stays advisory, no fix.
func searchCommented(xs []int, target int) (int, bool) {
	i, ok := slices.BinarySearchFunc(xs, target, func(a, b int) int { // want `slices\.BinarySearch searches the int elements with the identical \(index, found\) result and the comparison inlined \(no auto-fix: an explicit instantiation's three type arguments do not transfer to BinarySearch's two, or a comment in the deleted span\)`
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})
	fmt.Println(i, ok)
	return i, ok
}
