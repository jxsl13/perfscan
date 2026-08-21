package ps3107

import (
	"cmp"
	"fmt"
	"slices"
)

type Celsius int

// The bare-cmp.Compare comparator is slices.Sort spelled the slow way; the
// rewrites below delete this file's ONLY cmp references, so the fix also
// drops the orphaned cmp import. The slice expression is kept verbatim.
func sortInts(xs []int) {
	slices.SortFunc(xs, func(a, b int) int { return cmp.Compare(a, b) }) // want `slices\.SortFunc with a bare cmp\.Compare\(a, b\) comparator pays an indirect comparator call per comparison; slices\.Sort sorts the int elements with the identical ascending order and the comparison inlined`
	fmt.Println(xs)
}

func sortStrings(ys []string) {
	slices.SortFunc(ys, func(a, b string) int { return cmp.Compare(a, b) }) // want `slices\.Sort sorts the string elements with the identical ascending order and the comparison inlined`
	fmt.Println(ys)
}

// A NAMED ordered element is fixed too: sorting orders by the ordered
// value, equal values are identical bytes, and a String() method on the
// element is never consulted.
func sortNamed(cs []Celsius) {
	slices.SortFunc(cs, func(a, b Celsius) int { return cmp.Compare(a, b) }) // want `slices\.Sort sorts the Celsius elements with the identical ascending order and the comparison inlined`
	fmt.Println(cs)
}

// Parameter names are matched by object identity, not spelling, and the
// operand expression is kept verbatim, however it is spelled.
func sortField(w struct{ ids []uint64 }) {
	slices.SortFunc(w.ids, func(x, y uint64) int { return cmp.Compare(x, y) }) // want `slices\.Sort sorts the uint64 elements with the identical ascending order and the comparison inlined`
	fmt.Println(w.ids)
}

// The two-field parameter spelling func(a T, b T) matches like func(a, b T),
// and a deferred call rewrites the same way (both spellings evaluate the
// slice expression at defer time and sort at function exit).
func sortDeferred(xs []int) {
	defer slices.SortFunc(xs, func(a int, b int) int { return cmp.Compare(a, b) }) // want `slices\.Sort sorts the int elements with the identical ascending order and the comparison inlined`
	fmt.Println(xs)
}
