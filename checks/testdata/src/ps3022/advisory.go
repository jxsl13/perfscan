package ps3022

import (
	"cmp"
	"fmt"
	"slices"
)

// STRING elements are bit-identical (the ladder's sign equals cmp.Compare on
// strings and ties are identical byte content) but slices.Max/Min on a
// []string fold via an outlined runtime.strmax/strmin call per element —
// measured ~10-25% SLOWER on gc than the devirtualized hand-rolled loop.
// Advisory, no fix: never auto-recommend slower code.
func maxStrings(ys []string) string {
	return slices.MaxFunc(ys, func(a, b string) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Max computes the extremal string value with the comparison inlined \(no auto-fix: slices\.Max/Min on a string slice fold to an outlined runtime\.strmax/strmin call per element, ~10-25% slower on gc than the devirtualized hand-rolled loop; bit-identical but a measured perf regression\)`
}

// FLOAT elements: NaN compares neither '<' nor '>', so this comparator
// answers 0 for a NaN against ANYTHING — MaxFunc then keeps whichever value
// came first — while slices.Max PROPAGATES NaN via the builtin max. The two
// disagree on any slice containing NaN. Advisory, no fix.
func maxFloats(fs []float64) float64 {
	return slices.MaxFunc(fs, func(a, b float64) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Max computes the extremal float64 value with the comparison inlined \(no auto-fix: slices\.Max/Min propagate NaN via the builtin max/min while this comparator answers 0 for NaN against anything, so they differ on a NaN element\)`
}

// A TYPE-PARAMETER element may be instantiated with floats — the same NaN
// hazard — advisory, no fix.
func minAny[T cmp.Ordered](s []T) T {
	return slices.MinFunc(s, func(a, b T) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Min computes the extremal T value with the comparison inlined \(no auto-fix: type-parameter element, instantiations may include floats whose NaN handling differs\)`
}

// A comment inside the span the rewrite would delete — here the whole
// multi-line comparator body, this `want` comment included — would be
// silently destroyed: the report still fires, but stays advisory, no fix.
func maxCommented(xs []int) {
	m := slices.MaxFunc(xs, func(a, b int) int { // want `slices\.Max computes the extremal int value with the identical result and the comparison inlined`
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})
	fmt.Println(m)
}
