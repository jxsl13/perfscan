package ps3111

import (
	"cmp"
	"slices"
)

// FLOAT elements: advisory only (reported, NOT fixed) — slices.Max propagates
// NaN via the builtin max while cmp.Compare orders NaN first.
func maxFloat(fs []float64) float64 {
	return slices.MaxFunc(fs, cmp.Compare) // want `slices\.MaxFunc with a bare cmp\.Compare comparator`
}

// Swapped operands = the OPPOSITE extremum — never matched.
func swapped(xs []int) int {
	return slices.MaxFunc(xs, func(a, b int) int { return cmp.Compare(b, a) })
}

// A custom comparator, not cmp.Compare.
func custom(xs []int) int {
	return slices.MaxFunc(xs, func(a, b int) int {
		if a > b {
			return 1
		}
		return -1
	})
}

// Already the direct call.
func direct(xs []int) int {
	return slices.Max(xs)
}
