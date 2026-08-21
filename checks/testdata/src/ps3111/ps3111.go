package ps3111

import (
	"cmp"
	"slices"
)

type Priority int

// The bare cmp.Compare comparator is slices.Max/Min spelled the slow way. The
// integer maxInt/maxNamed are fixed; the string minStr stays ADVISORY (slices.Max/
// Min fold via an outlined runtime.strmax and regress ~10-25% on []string), so it
// keeps cmp.Compare and the cmp import is retained. Slice expressions kept verbatim.

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
