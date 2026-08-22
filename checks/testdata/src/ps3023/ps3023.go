package ps3023

import (
	"cmp"
	"slices"
)

type Rank int

// The bare cmp.Compare comparator is slices.Compare spelled the slow way.
// Every case here is FIXED — floats included (slices.Compare calls cmp.Compare
// per pair internally, so NaN and -0.0 agree by construction, unlike the
// Max/Min family) — so the rewrites remove the file's last cmp references and
// the golden drops the cmp import. Slice expressions kept verbatim.

// cmp.Compare passed directly.
func ints(a, b []int) int {
	return slices.CompareFunc(a, b, cmp.Compare) // want `slices\.CompareFunc with a bare cmp\.Compare comparator`
}

// A func literal wrapping cmp.Compare, parameters in source order.
func strs(a, b []string) bool {
	return slices.CompareFunc(a, b, func(x, y string) int { return cmp.Compare(x, y) }) < 0 // want `slices\.CompareFunc with a bare cmp\.Compare comparator`
}

// A named ordered element is fixed too.
func ranks(a, b []Rank) int {
	return slices.CompareFunc(a, b, func(x, y Rank) int { return cmp.Compare(x, y) }) // want `slices\.CompareFunc with a bare cmp\.Compare comparator`
}

// FLOAT elements are fixed — slices.Compare uses cmp.Compare internally, so
// NaN ordering and -0.0 ties are identical on both sides (contrast PS3111,
// where the builtin max propagates NaN and floats stay advisory).
func floats(a, b []float64) int {
	return slices.CompareFunc(a, b, cmp.Compare) // want `slices\.CompareFunc with a bare cmp\.Compare comparator`
}
