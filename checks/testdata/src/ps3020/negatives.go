package ps3020

import (
	"cmp"
	"slices"
)

// FLOAT elements: advisory only (reported, NOT fixed) — slices.Max propagates
// NaN via the builtin max while the swapped MinFunc scan never selects one,
// and signed zeros tie under cmp.Compare but not under the builtin min/max.
func highestFloat(fs []float64) float64 {
	return slices.MinFunc(fs, func(a, b float64) int { return cmp.Compare(b, a) }) // want `slices\.MinFunc with a swapped cmp\.Compare\(b, a\) comparator selects the maximum`
}

// STRING elements: advisory only — slices.Min on []string folds to an
// outlined runtime.strmin call per element, a measured regression on gc.
func lowestStr(ys []string) string {
	return slices.MaxFunc(ys, func(a, b string) int { return cmp.Compare(b, a) }) // want `slices\.MaxFunc with a swapped cmp\.Compare\(b, a\) comparator selects the minimum`
}

// TYPE-PARAMETER elements: advisory only — an instantiation may be a float.
func lowestOf[T cmp.Ordered](xs []T) T {
	return slices.MaxFunc(xs, func(a, b T) int { return cmp.Compare(b, a) }) // want `slices\.MaxFunc with a swapped cmp\.Compare\(b, a\) comparator selects the minimum`
}

// A comment inside the deleted span would be destroyed — advisory, no fix.
func commented(xs []int) int {
	return slices.MaxFunc(xs, /* keep the reversed order! */ func(a, b int) int { return cmp.Compare(b, a) }) // want `slices\.MaxFunc with a swapped cmp\.Compare\(b, a\) comparator selects the minimum`
}

// Source-order cmp.Compare(a, b) is the SAME extremum — PS3111's case, never
// matched here.
func sourceOrder(xs []int) int {
	return slices.MaxFunc(xs, func(a, b int) int { return cmp.Compare(a, b) })
}

// A bare cmp.Compare func value is always the source order — PS3111's case.
func bare(xs []int) int {
	return slices.MaxFunc(xs, cmp.Compare)
}

// Repeating one parameter is not the swapped order.
func repeated(xs []int) int {
	return slices.MaxFunc(xs, func(a, b int) int { return cmp.Compare(b, b) })
}

// A custom comparator, not cmp.Compare.
func custom(xs []int) int {
	return slices.MaxFunc(xs, func(a, b int) int {
		if a > b {
			return -1
		}
		return 1
	})
}

// Extra work around the call fails the exact match.
func negated(xs []int) int {
	return slices.MaxFunc(xs, func(a, b int) int { return -cmp.Compare(a, b) })
}

// Already the direct call.
func direct(xs []int) int {
	return slices.Min(xs)
}
