package ps3035

import (
	"fmt"
	"slices"
)

type Priority int

// The swapped hand-rolled three-way (a>b -> negative, a<b -> positive)
// reverses the order, so MaxFunc selects the MINIMUM and MinFunc the MAXIMUM;
// every integer site is fixed to the OPPOSITE direct extremum and the slice
// expression is kept verbatim. (The fixture comparators are one-liners
// because the `want` comment must share the call's line, and a comment
// INSIDE the deleted span keeps a report advisory — see advisory.go; the AST
// shapes are identical to the multi-line spellings.)
func lowest(xs []int) int {
	return slices.MaxFunc(xs, func(a, b int) int { if a > b { return -1 }; if a < b { return 1 }; return 0 }) // want `slices\.MaxFunc with a swapped hand-rolled three-way comparator \(a>b/-1, a<b/1\) selects the minimum through an indirect comparator call plus up to two relational comparisons per element; slices\.Min computes the identical int minimum with the comparison inlined`
}

// MinFunc under the reversed order is the maximum; the if/else-if chain with
// a trailing return is the same swapped three-way.
func highest(xs []int) int {
	return slices.MinFunc(xs, func(a, b int) int { if a > b { return -1 } else if a < b { return 1 }; return 0 }) // want `slices\.MinFunc with a swapped hand-rolled three-way comparator \(a>b/-1, a<b/1\) selects the maximum through an indirect comparator call plus up to two relational comparisons per element; slices\.Max computes the identical int maximum with the comparison inlined`
}

// The fully chained if/else-if/else spelling maps identically, with the
// "less" arm first (branch order is free; only direction+sign are checked).
func lowestChained(xs []uint32) uint32 {
	return slices.MaxFunc(xs, func(a, b uint32) int { if a < b { return 1 } else if a > b { return -1 } else { return 0 } }) // want `slices\.Min computes the identical uint32 minimum with the comparison inlined`
}

// An expressionless switch with a default clause is the same swapped
// three-way, and a NAMED integer element is fixed too: equal values are
// identical bytes and no method of the element is ever consulted.
func highestNamed(ps []Priority) Priority {
	return slices.MinFunc(ps, func(a, b Priority) int { switch { case a > b: return -1; case a < b: return 1; default: return 0 } }) // want `slices\.Max computes the identical Priority maximum with the comparison inlined`
}

// A switch without a default clause plus the trailing return matches too;
// parameter names are matched by object identity, not spelling, and the
// operand expression is kept verbatim, however it is spelled.
func lowestField(w struct{ ids []uint64 }) uint64 {
	return slices.MaxFunc(w.ids, func(x, y uint64) int { switch { case x > y: return -1; case x < y: return 1 }; return 0 }) // want `slices\.Min computes the identical uint64 minimum with the comparison inlined`
}

// The b</b> operand spellings (b<a is "greater", b>a is "less") and
// magnitudes other than 1 are the same swapped three-way — only the SIGN is
// consumed. The two-field parameter spelling func(a T, b T) matches like
// func(a, b T), and a unary +7 is a positive literal like 1.
func highestSwapped(xs []int) int {
	return slices.MinFunc(xs, func(a int, b int) int { if b < a { return -42 }; if b > a { return +7 }; return 0 }) // want `slices\.Max computes the identical int maximum with the comparison inlined`
}

// A statement-position call rewrites the same way.
func lowestDiscarded(xs []int8) {
	slices.MaxFunc(xs, func(a, b int8) int { if a > b { return -1 }; if a < b { return 1 }; return 0 }) // want `slices\.Min computes the identical int8 minimum with the comparison inlined`
	fmt.Println(xs)
}
