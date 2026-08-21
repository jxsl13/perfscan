package ps3019

import (
	"fmt"
	"slices"
)

type Celsius int

// The hand-rolled three-way if-chain is slices.IsSorted spelled the slow
// way; the slice expression is kept verbatim. (The fixture comparators are
// one-liners because the `want` comment must share the call's line, and a
// comment INSIDE the deleted span keeps a report advisory — see
// advisory.go; the AST shapes are identical to the multi-line spellings.)
func checkInts(xs []int) bool {
	return slices.IsSortedFunc(xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.IsSortedFunc with a hand-rolled three-way comparator \(a<b/a>b/-1/1/0\) pays an indirect comparator call plus up to two relational comparisons per adjacent pair; slices\.IsSorted answers the identical bool over the int elements with a single inlined comparison`
}

// The if/else-if chain with a trailing return is the same three-way, and
// strings order by raw bytes on both sides.
func checkStrings(ys []string) bool {
	return slices.IsSortedFunc(ys, func(a, b string) int { if a < b { return -1 } else if a > b { return 1 }; return 0 }) // want `slices\.IsSorted answers the identical bool over the string elements with a single inlined comparison`
}

// The fully chained if/else-if/else spelling maps identically.
func checkChained(xs []int) {
	if slices.IsSortedFunc(xs, func(a, b int) int { if a < b { return -1 } else if a > b { return 1 } else { return 0 } }) { // want `slices\.IsSorted answers the identical bool over the int elements with a single inlined comparison`
		fmt.Println("sorted")
	}
}

// An expressionless switch with a default clause is the same three-way, and
// a NAMED ordered element is fixed too: the scan compares the ordered
// values, and a String() method on the element is never consulted.
func checkNamed(cs []Celsius) bool {
	return slices.IsSortedFunc(cs, func(a, b Celsius) int { switch { case a < b: return -1; case a > b: return 1; default: return 0 } }) // want `slices\.IsSorted answers the identical bool over the Celsius elements with a single inlined comparison`
}

// A switch without a default clause plus the trailing return matches too;
// parameter names are matched by object identity, not spelling, and the
// operand expression is kept verbatim, however it is spelled.
func checkField(w struct{ ids []uint64 }) bool {
	return slices.IsSortedFunc(w.ids, func(x, y uint64) int { switch { case x < y: return -1; case x > y: return 1 }; return 0 }) // want `slices\.IsSorted answers the identical bool over the uint64 elements with a single inlined comparison`
}

// The two ifs in SWAPPED order (greater first), the b</b> operand
// spellings, and magnitudes other than 1 are the same three-way — only the
// SIGN is consumed. The two-field parameter spelling func(a T, b T)
// matches like func(a, b T).
func checkSwapped(xs []int) bool {
	return slices.IsSortedFunc(xs, func(a int, b int) int { if b < a { return 42 }; if b > a { return -7 }; return 0 }) // want `slices\.IsSorted answers the identical bool over the int elements with a single inlined comparison`
}

// The result feeding another expression rewrites the same way; a unary +1
// is a positive literal like 1.
func checkNegated(xs []int) bool {
	return !slices.IsSortedFunc(xs, func(a, b int) int { if a < b { return -1 }; if a > b { return +1 }; return 0 }) // want `slices\.IsSorted answers the identical bool over the int elements with a single inlined comparison`
}
