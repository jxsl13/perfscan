package ps3022

import (
	"fmt"
	"slices"
)

type Priority int

// The hand-rolled three-way ladder is slices.Max/Min spelled the slow way;
// the slice expression is kept verbatim. (The fixture comparators are
// one-liners because the `want` comment must share the call's line, and a
// comment INSIDE the deleted span keeps a report advisory — see advisory.go;
// the AST shapes are identical to the multi-line spellings.)
func maxInts(xs []int) int {
	return slices.MaxFunc(xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.MaxFunc with a hand-rolled three-way comparator \(a<b/a>b/-1/1/0\) pays an indirect comparator call plus up to two relational comparisons per element; slices\.Max computes the extremal int value with the identical result and the comparison inlined`
}

// MinFunc is the symmetric pair; the if/else-if chain with a trailing return
// is the same three-way.
func minInts(xs []int) int {
	return slices.MinFunc(xs, func(a, b int) int { if a < b { return -1 } else if a > b { return 1 }; return 0 }) // want `slices\.MinFunc with a hand-rolled three-way comparator \(a<b/a>b/-1/1/0\) pays an indirect comparator call plus up to two relational comparisons per element; slices\.Min computes the extremal int value with the identical result and the comparison inlined`
}

// The fully chained if/else-if/else spelling maps identically.
func maxChained(xs []uint32) uint32 {
	return slices.MaxFunc(xs, func(a, b uint32) int { if a < b { return -1 } else if a > b { return 1 } else { return 0 } }) // want `slices\.Max computes the extremal uint32 value with the identical result and the comparison inlined`
}

// An expressionless switch with a default clause is the same three-way, and
// a NAMED integer element is fixed too: equal values are identical bytes and
// no method of the element is ever consulted.
func maxNamed(ps []Priority) Priority {
	return slices.MaxFunc(ps, func(a, b Priority) int { switch { case a < b: return -1; case a > b: return 1; default: return 0 } }) // want `slices\.Max computes the extremal Priority value with the identical result and the comparison inlined`
}

// A switch without a default clause plus the trailing return matches too;
// parameter names are matched by object identity, not spelling, and the
// operand expression is kept verbatim, however it is spelled.
func minField(w struct{ ids []uint64 }) uint64 {
	return slices.MinFunc(w.ids, func(x, y uint64) int { switch { case x < y: return -1; case x > y: return 1 }; return 0 }) // want `slices\.Min computes the extremal uint64 value with the identical result and the comparison inlined`
}

// The two ifs in SWAPPED order (greater first), the b</b> operand spellings,
// and magnitudes other than 1 are the same three-way — only the SIGN is
// consumed. The two-field parameter spelling func(a T, b T) matches like
// func(a, b T), and a unary +1 is a positive literal like 1.
func maxSwapped(xs []int) int {
	return slices.MaxFunc(xs, func(a int, b int) int { if b < a { return +42 }; if b > a { return -7 }; return 0 }) // want `slices\.Max computes the extremal int value with the identical result and the comparison inlined`
}

// A statement-position call rewrites the same way.
func maxDiscarded(xs []int8) {
	slices.MaxFunc(xs, func(a, b int8) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Max computes the extremal int8 value with the identical result and the comparison inlined`
	fmt.Println(xs)
}
