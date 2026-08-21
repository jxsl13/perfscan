package ps3010

import (
	"cmp"
	"slices"
)

type Priority int

type Temp float64

// The bare cmp.Compare comparator is slices.IsSorted spelled the slow way; the
// rewrites delete this file's ONLY cmp references, so the fix drops the
// orphaned cmp import. The slice expression is kept verbatim.

// cmp.Compare passed directly.
func sortedInt(xs []int) bool {
	return slices.IsSortedFunc(xs, cmp.Compare) // want `slices\.IsSortedFunc with a bare cmp\.Compare comparator`
}

// A func literal wrapping cmp.Compare.
func sortedStr(ys []string) bool {
	return slices.IsSortedFunc(ys, func(a, b string) int { return cmp.Compare(a, b) }) // want `slices\.IsSortedFunc with a bare cmp\.Compare comparator`
}

// FLOAT elements are FIXED here, unlike the sort-family siblings: the result
// is a pure bool and cmp.Compare(a, b) < 0 iff cmp.Less(a, b) for every float
// input (NaN and ±0.0 included), so there is no observable tie arrangement.
func sortedFloat(fs []float64) bool {
	return slices.IsSortedFunc(fs, cmp.Compare) // want `slices\.IsSortedFunc with a bare cmp\.Compare comparator`
}

// A named ordered element is fixed too — including a named float.
func sortedNamed(ps []Priority, ts []Temp) bool {
	return slices.IsSortedFunc(ps, func(a, b Priority) int { return cmp.Compare(a, b) }) && // want `slices\.IsSortedFunc with a bare cmp\.Compare comparator`
		slices.IsSortedFunc(ts, cmp.Compare) // want `slices\.IsSortedFunc with a bare cmp\.Compare comparator`
}

// An explicit instantiation keeps its brackets: slices.IsSorted has the same
// two type parameters.
func sortedInstantiated(xs []int) bool {
	return slices.IsSortedFunc[[]int, int](xs, cmp.Compare) // want `slices\.IsSortedFunc with a bare cmp\.Compare comparator`
}
