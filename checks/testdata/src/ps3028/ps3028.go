package ps3028

import (
	"fmt"
	"slices"
)

type Version int

// The hand-rolled three-way if-chain is slices.BinarySearch spelled the slow
// way; the slice and target expressions are kept verbatim. (The fixture
// comparators are one-liners because the `want` comment must share the call's
// line, and a comment INSIDE the deleted span keeps a report advisory — see
// advisory.go; the AST shapes are identical to the multi-line spellings.)
func searchInts(xs []int, target int) (int, bool) {
	return slices.BinarySearchFunc(xs, target, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.BinarySearchFunc with a hand-rolled three-way comparator \(a<b/a>b/-1/1/0\) pays an indirect comparator call plus up to two relational comparisons per probe; slices\.BinarySearch searches the int elements with the identical \(index, found\) result and the comparison inlined`
}

// STRING elements are fixed too (PS3013's policy): no NaN, so the chain's
// sign is the search's own ordering on every probe. The if/else-if chain with
// a trailing return is the same three-way.
func searchStrings(ys []string, target string) (int, bool) {
	return slices.BinarySearchFunc(ys, target, func(a, b string) int { if a < b { return -1 } else if a > b { return 1 }; return 0 }) // want `slices\.BinarySearch searches the string elements with the identical \(index, found\) result and the comparison inlined`
}

// The fully chained if/else-if/else spelling maps identically.
func searchChained(xs []int, target int) (int, bool) {
	return slices.BinarySearchFunc(xs, target, func(a, b int) int { if a < b { return -1 } else if a > b { return 1 } else { return 0 } }) // want `slices\.BinarySearch searches the int elements with the identical \(index, found\) result and the comparison inlined`
}

// An expressionless switch with a default clause is the same three-way, and a
// NAMED ordered element is fixed too: ~int satisfies cmp.Ordered and no
// method of the element is ever consulted.
func searchNamed(vs []Version, target Version) (int, bool) {
	return slices.BinarySearchFunc(vs, target, func(a, b Version) int { switch { case a < b: return -1; case a > b: return 1; default: return 0 } }) // want `slices\.BinarySearch searches the Version elements with the identical \(index, found\) result and the comparison inlined`
}

// A switch without a default clause plus the trailing return matches too;
// parameter names are matched by object identity, not spelling, and the slice
// and target expressions are kept verbatim, however they are spelled.
func searchField(w struct{ ids []uint64 }, t uint64) (int, bool) {
	return slices.BinarySearchFunc(w.ids, t+1, func(x, y uint64) int { switch { case x < y: return -1; case x > y: return 1 }; return 0 }) // want `slices\.BinarySearch searches the uint64 elements with the identical \(index, found\) result and the comparison inlined`
}

// The two arms in SWAPPED order (greater first), the b</b> operand spellings,
// and magnitudes other than 1 are the same three-way — BinarySearchFunc
// consumes only the SIGN ('< 0' probes, '== 0' found), unlike CompareFunc's
// verbatim value propagation (PS3027). The two-field parameter spelling
// func(a T, b T) matches like func(a, b T).
func searchSwapped(xs []int, target int) (int, bool) {
	return slices.BinarySearchFunc(xs, target, func(a int, b int) int { if b < a { return 42 }; if b > a { return -7 }; return 0 }) // want `slices\.BinarySearch searches the int elements with the identical \(index, found\) result and the comparison inlined`
}

// A unary +1 is a positive literal like 1, and the result feeding directly
// into other expressions rewrites the same way.
func searchUsed(xs []int, target int) bool {
	_, ok := slices.BinarySearchFunc(xs, target, func(a, b int) int { if a < b { return -1 }; if a > b { return +1 }; return 0 }) // want `slices\.BinarySearch searches the int elements with the identical \(index, found\) result and the comparison inlined`
	fmt.Println(ok)
	return ok
}
