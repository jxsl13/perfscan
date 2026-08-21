package ps3026

import (
	"fmt"
	"maps"
	"slices"
)

type Celsius int

// The hand-rolled three-way if-chain is slices.Sorted spelled the slow way;
// the seq expression is kept verbatim. (The fixture comparators are
// one-liners because the `want` comment must share the call's line, and a
// comment INSIDE the deleted span keeps a report advisory — see advisory.go;
// the AST shapes are identical to the multi-line spellings.)
func sortedVals(m map[string]int) []int {
	return slices.SortedFunc(maps.Values(m), func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.SortedFunc with a hand-rolled three-way comparator \(a<b/a>b/-1/1/0\) pays an indirect comparator call plus up to two relational comparisons per comparison; slices\.Sorted collects and sorts the int elements with the identical ascending order and a single inlined comparison`
}

// The if/else-if chain with a trailing return is the same three-way, and
// STRING elements are fixable too (byte-identical, and the monomorphized
// sort is measurably faster — the same policy as PS3012/PS3013).
func sortedKeys(m map[string]int) []string {
	return slices.SortedFunc(maps.Keys(m), func(a, b string) int { if a < b { return -1 } else if a > b { return 1 }; return 0 }) // want `slices\.Sorted collects and sorts the string elements with the identical ascending order and a single inlined comparison`
}

// The fully chained if/else-if/else spelling maps identically.
func sortedChained(xs []int) []int {
	return slices.SortedFunc(slices.Values(xs), func(a, b int) int { if a < b { return -1 } else if a > b { return 1 } else { return 0 } }) // want `slices\.Sorted collects and sorts the int elements with the identical ascending order and a single inlined comparison`
}

// An expressionless switch with a default clause is the same three-way, and
// a NAMED ordered element is fixed too: sorting orders by the ordered value,
// equal values are identical bytes, and a String() method on the element is
// never consulted.
func sortedNamed(cs []Celsius) []Celsius {
	return slices.SortedFunc(slices.Values(cs), func(a, b Celsius) int { switch { case a < b: return -1; case a > b: return 1; default: return 0 } }) // want `slices\.Sorted collects and sorts the Celsius elements with the identical ascending order and a single inlined comparison`
}

// A switch without a default clause plus the trailing return matches too;
// parameter names are matched by object identity, not spelling, and the seq
// expression is kept verbatim, however it is spelled.
func sortedField(w struct{ ids map[int]uint64 }) []uint64 {
	return slices.SortedFunc(maps.Values(w.ids), func(x, y uint64) int { switch { case x < y: return -1; case x > y: return 1 }; return 0 }) // want `slices\.Sorted collects and sorts the uint64 elements with the identical ascending order and a single inlined comparison`
}

// The two arms in SWAPPED order (greater first), the b</b> operand
// spellings, and magnitudes other than 1 are the same three-way — only the
// SIGN is consumed. The two-field parameter spelling func(a T, b T) matches
// like func(a, b T), and a unary +1 is a positive literal like 1.
func sortedSwapped(xs []int) {
	fmt.Println(slices.SortedFunc(slices.Values(xs), func(a int, b int) int { if b < a { return 42 }; if b > a { return -7 }; return 0 })) // want `slices\.Sorted collects and sorts the int elements with the identical ascending order and a single inlined comparison`
	fmt.Println(slices.SortedFunc(slices.Values(xs), func(a, b int) int { if a < b { return -1 }; if a > b { return +1 }; return 0 }))     // want `slices\.Sorted collects and sorts the int elements with the identical ascending order and a single inlined comparison`
}
