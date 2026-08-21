package ps3035

import (
	"cmp"
	"fmt"
	"slices"
)

// STRING elements are bit-identical (the swapped ladder's sign equals
// cmp.Compare(b, a) on strings and ties are identical byte content) but
// slices.Min/Max on a []string fold via an outlined runtime.strmin/strmax
// call per element — measured ~10-25% SLOWER on gc than the devirtualized
// hand-rolled loop. Advisory, no fix: never auto-recommend slower code.
func lowestStrings(ys []string) string {
	return slices.MaxFunc(ys, func(a, b string) int { if a > b { return -1 }; if a < b { return 1 }; return 0 }) // want `slices\.Min computes the string minimum with the comparison inlined \(no auto-fix: slices\.Min/Max on a string slice fold to an outlined runtime\.strmin/strmax call per element, ~10-25% slower on gc than the devirtualized hand-rolled loop; bit-identical but a measured perf regression\)`
}

// FLOAT elements: NaN compares neither '>' nor '<', so this comparator
// answers 0 for a NaN against ANYTHING — the Func scan then keeps whichever
// value came first — and it calls -0.0/+0.0 a tie, while slices.Min
// PROPAGATES NaN via the builtin min and orders -0.0 below +0.0. The two
// disagree on any slice containing NaN or mixed-sign zeros. Advisory, no fix.
func lowestFloats(fs []float64) float64 {
	return slices.MaxFunc(fs, func(a, b float64) int { if a > b { return -1 }; if a < b { return 1 }; return 0 }) // want `slices\.Min computes the float64 minimum with the comparison inlined \(no auto-fix: slices\.Min/Max propagate NaN and order signed zeros via the builtin min/max while this comparator answers 0 for NaN against anything and calls -0\.0/\+0\.0 a tie, so they differ on a NaN or signed-zero element\)`
}

// A TYPE-PARAMETER element may be instantiated with floats — the same NaN
// and signed-zero hazard — advisory, no fix.
func highestAny[T cmp.Ordered](s []T) T {
	return slices.MinFunc(s, func(a, b T) int { if a > b { return -1 }; if a < b { return 1 }; return 0 }) // want `slices\.Max computes the T maximum with the comparison inlined \(no auto-fix: type-parameter element, instantiations may include floats whose NaN and signed-zero handling differ\)`
}

// A comment inside the span the rewrite would delete — here the whole
// multi-line comparator body, this `want` comment included — would be
// silently destroyed: the report still fires, but stays advisory, no fix.
func lowestCommented(xs []int) {
	m := slices.MaxFunc(xs, func(a, b int) int { // want `slices\.MaxFunc with a swapped hand-rolled three-way comparator \(a>b/-1, a<b/1\) selects the minimum through an indirect comparator call plus up to two relational comparisons per element; slices\.Min computes the identical int minimum with the comparison inlined`
		if a > b {
			return -1
		}
		if a < b {
			return 1
		}
		return 0
	})
	fmt.Println(m)
}
