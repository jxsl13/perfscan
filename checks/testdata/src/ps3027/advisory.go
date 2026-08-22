package ps3027

import (
	"cmp"
	"fmt"
	"slices"
)

type IntSlice []int

// NON-UNIT MAGNITUDES: sort.Slice and slices.MaxFunc read only the SIGN of
// the comparator's result, but slices.CompareFunc returns the first non-zero
// result VERBATIM — this call yields -2/+3 where slices.Compare yields the
// canonical -1/+1. The relaxation the sign-only siblings allow is unsound
// here. Advisory, no fix.
func compareLoose(a, b []int) int {
	return slices.CompareFunc(a, b, func(x, y int) int { if x < y { return -2 }; if x > y { return 3 }; return 0 }) // want `slices\.CompareFunc with a hand-rolled three-way comparator \(a<b/a>b/-1/1/0\) pays an indirect comparator call plus up to two relational comparisons per element pair; slices\.Compare compares the int elements with the comparison inlined \(no auto-fix: slices\.CompareFunc returns the comparator's first non-zero result verbatim, so this ladder's non-±1 magnitudes reach the caller while slices\.Compare would return the canonical -1/\+1 — the returned value, not just its sign, would change\)`
}

// FLOAT elements: NaN compares neither '<' nor '>', so this comparator
// answers 0 for a NaN against ANYTHING — CompareFunc treats the position as
// a tie and scans on — while slices.Compare orders NaN first via cmp.Compare
// ([]float64{NaN} vs []float64{1} is 0 vs -1). Advisory, no fix.
func compareFloats(a, b []float64) int {
	return slices.CompareFunc(a, b, func(x, y float64) int { if x < y { return -1 }; if x > y { return 1 }; return 0 }) // want `slices\.Compare compares the float64 elements with the comparison inlined \(no auto-fix: slices\.Compare orders NaN first via cmp\.Compare while this comparator answers 0 for NaN against anything, so they differ on a NaN element\)`
}

// STRING elements are bit-identical (no NaN, the ladder IS cmp.Compare) but
// stay advisory: the generic cmp.Compare instantiation inside slices.Compare
// performs two isNaN self-comparisons per pair that the devirtualized ladder
// omits — measured parity at best, so there is no win to auto-buy.
func compareStrings(a, b []string) int {
	return slices.CompareFunc(a, b, func(x, y string) int { if x < y { return -1 }; if x > y { return 1 }; return 0 }) // want `slices\.Compare compares the string elements with the identical lexicographic result and the comparison inlined \(no auto-fix: bit-identical for strings, but slices\.Compare's generic cmp\.Compare instantiation adds two isNaN self-comparisons per pair that this devirtualized ladder omits — measured parity at best, no win to buy, matching the family's string carve-outs\)`
}

// A TYPE-PARAMETER element may be instantiated with floats — the same NaN
// hazard — advisory, no fix.
func compareAny[T cmp.Ordered](a, b []T) int {
	return slices.CompareFunc(a, b, func(x, y T) int { if x < y { return -1 }; if x > y { return 1 }; return 0 }) // want `slices\.Compare compares the T elements with the comparison inlined \(no auto-fix: type-parameter element, instantiations may include floats whose NaN handling differs\)`
}

// MIXED SLICE TYPES: CompareFunc takes two independently-typed slices
// (S1, S2) while slices.Compare takes one S for both — []int vs IntSlice
// compiles only in the Func spelling. Advisory, no fix.
func compareMixed(a []int, b IntSlice) int {
	return slices.CompareFunc(a, b, func(x, y int) int { if x < y { return -1 }; if x > y { return 1 }; return 0 }) // want `slices\.Compare compares the int elements with the identical lexicographic result and the comparison inlined \(no auto-fix: the slice types \[\]int and IntSlice differ — slices\.Compare takes one slice type for both sides\)`
}

// EXPLICIT INSTANTIATION: CompareFunc has four type parameters, Compare two,
// so the brackets cannot survive the rewrite. Advisory, no fix.
func compareInstantiated(a, b []int) int {
	return slices.CompareFunc[[]int, []int, int, int](a, b, func(x, y int) int { if x < y { return -1 }; if x > y { return 1 }; return 0 }) // want `slices\.Compare compares the int elements with the identical lexicographic result and the comparison inlined \(no auto-fix: explicit instantiation — slices\.CompareFunc has four type parameters, slices\.Compare two, so the brackets cannot survive the rewrite\)`
}

// A comment inside the span the rewrite would delete — here the whole
// multi-line comparator body, this `want` comment included — would be
// silently destroyed: the report still fires, but stays advisory, no fix.
func compareCommented(a, b []int) {
	d := slices.CompareFunc(a, b, func(x, y int) int { // want `slices\.Compare compares the int elements with the identical lexicographic result and the comparison inlined`
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
		return 0
	})
	fmt.Println(d)
}
