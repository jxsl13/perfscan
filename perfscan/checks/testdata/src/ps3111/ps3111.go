package ps3111

import (
	"cmp"
	"slices"
)

type Priority int

// The bare cmp.Compare comparator is slices.Max/Min spelled the slow way; the
// rewrites delete this file's ONLY cmp references, so the fix drops the orphaned
// cmp import. The slice expression is kept verbatim.

// cmp.Compare passed directly.
func maxInt(xs []int) int {
	return slices.MaxFunc(xs, cmp.Compare) // want `slices\.MaxFunc with a bare cmp\.Compare comparator`
}

// A func literal wrapping cmp.Compare.
func minStr(ys []string) string {
	return slices.MinFunc(ys, func(a, b string) int { return cmp.Compare(a, b) }) // want `slices\.MinFunc with a bare cmp\.Compare comparator`
}

// A named ordered element is fixed too.
func maxNamed(ps []Priority) Priority {
	return slices.MaxFunc(ps, func(a, b Priority) int { return cmp.Compare(a, b) }) // want `slices\.MaxFunc with a bare cmp\.Compare comparator`
}
