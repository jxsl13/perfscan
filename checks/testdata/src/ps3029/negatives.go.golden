package ps3029

import (
	"fmt"
	"math"
	"slices"
)

type item struct{ id int }

type fakeSlices struct{}

func (fakeSlices) SortedStableFunc(seq func(func(int) bool), f func(a, b int) int) []int { return nil }

const negOne = -1

var namedCmp = func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }

// None of these is the exact ascending hand-rolled three-way under
// slices.SortedStableFunc; none is reported, and none is rewritten.
func negatives(xs []int, ys []item, base int) {
	// The subtraction comparator can OVERFLOW (a-b for a=MinInt, b=1) and
	// is explicitly not matched.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { return a - b }))

	// Signs swapped: a<b returning POSITIVE is a DESCENDING sort — never
	// slices.Sorted.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { if a < b { return 1 }; if a > b { return -1 }; return 0 }))

	// '<=' is not the three-way: it answers -1 for EQUAL pairs, an
	// inconsistent comparator.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { if a <= b { return -1 }; if a > b { return 1 }; return 0 }))

	// A nonzero default is not the three-way.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 1 }))

	// SAME direction twice (b > a is a<b again): a>b falls through to 0 —
	// not a three-way, and not slices.Sorted's order.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { if a < b { return -1 }; if b > a { return 1 }; return 0 }))

	// A NAMED constant (math.MinInt) instead of a literal: deleting the
	// comparator would orphan the math import, so the shape is not matched
	// even though the sign is right.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { if a < b { return math.MinInt }; if a > b { return 1 }; return 0 }))

	// A local named constant is equally opaque to the literal-only shape.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { if a < b { return negOne }; if a > b { return 1 }; return 0 }))

	// A captured variable instead of the second parameter.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { if a < base { return -1 }; if a > base { return 1 }; return 0 }))

	// Field selectors: the comparison is not on the bare parameters (a
	// keyed sort whose struct ties the STABLE sort keeps observably in
	// order — never touched).
	fmt.Println(slices.SortedStableFunc(slices.Values(ys), func(a, b item) int { if a.id < b.id { return -1 }; if a.id > b.id { return 1 }; return 0 }))

	// Extra work in the body: not the bare three-way.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { fmt.Println(a, b); if a < b { return -1 }; if a > b { return 1 }; return 0 }))

	// A repeated parameter (a < a) is not a comparison of the two.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { if a < a { return -1 }; if a > b { return 1 }; return 0 }))

	// A tagged switch is not the expressionless three-way switch.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { switch true { case a < b: return -1; case a > b: return 1 }; return 0 }))

	// A named comparator value is opaque — only a fresh literal matches.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), namedCmp))

	// SortedFunc is a different callee (the unstable collecting sort);
	// PS3026's, not this check's.
	fmt.Println(slices.SortedFunc(slices.Values(xs), func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }))

	// Already the direct call.
	fmt.Println(slices.Sorted(slices.Values(xs)))
}

// A shadowed slices resolves SortedStableFunc to a METHOD of the local
// object, not the stdlib package function — never matched.
func shadowedSlices(seq func(func(int) bool)) []int {
	slices := fakeSlices{}
	return slices.SortedStableFunc(seq, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 })
}
