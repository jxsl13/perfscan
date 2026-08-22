package ps3019

import (
	"fmt"
	"math"
	"slices"
)

type item struct{ id int }

type fakeSlices struct{}

func (fakeSlices) IsSortedFunc(s []int, f func(a, b int) int) bool { return false }

const negOne = -1

var namedCmp = func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }

// None of these is the exact ascending hand-rolled three-way; none is
// reported, and none is rewritten.
func negatives(xs []int, ys []item, base int) {
	// The subtraction comparator can OVERFLOW (a-b for a=MinInt, b=1) and
	// is explicitly not matched.
	_ = slices.IsSortedFunc(xs, func(a, b int) int { return a - b })

	// Signs swapped: a<b returning POSITIVE asks whether xs is DESCENDING
	// — never slices.IsSorted.
	_ = slices.IsSortedFunc(xs, func(a, b int) int { if a < b { return 1 }; if a > b { return -1 }; return 0 })

	// '<=' is not the three-way: it answers -1 for EQUAL pairs, so equal
	// neighbors would read as out-of-order — a different predicate.
	_ = slices.IsSortedFunc(xs, func(a, b int) int { if a <= b { return -1 }; if a > b { return 1 }; return 0 })

	// A nonzero default is not the three-way.
	_ = slices.IsSortedFunc(xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 1 })

	// SAME direction twice (b > a is a<b again): a>b falls through to 0 —
	// not a three-way, and not slices.IsSorted's predicate.
	_ = slices.IsSortedFunc(xs, func(a, b int) int { if a < b { return -1 }; if b > a { return 1 }; return 0 })

	// A NAMED constant (math.MinInt) instead of a literal: deleting the
	// comparator would orphan the math import, so the shape is not matched
	// even though the sign is right.
	_ = slices.IsSortedFunc(xs, func(a, b int) int { if a < b { return math.MinInt }; if a > b { return 1 }; return 0 })

	// A local named constant is equally opaque to the literal-only shape.
	_ = slices.IsSortedFunc(xs, func(a, b int) int { if a < b { return negOne }; if a > b { return 1 }; return 0 })

	// A captured variable instead of the second parameter.
	_ = slices.IsSortedFunc(xs, func(a, b int) int { if a < base { return -1 }; if a > base { return 1 }; return 0 })

	// Field selectors: the comparison is not on the bare parameters, and
	// slices.IsSorted on the structs would not compile anyway.
	_ = slices.IsSortedFunc(ys, func(a, b item) int { if a.id < b.id { return -1 }; if a.id > b.id { return 1 }; return 0 })

	// Extra work in the body: not the bare three-way, and the side effect
	// would be lost.
	_ = slices.IsSortedFunc(xs, func(a, b int) int { fmt.Println(a, b); if a < b { return -1 }; if a > b { return 1 }; return 0 })

	// A repeated parameter (a < a) is not a comparison of the two.
	_ = slices.IsSortedFunc(xs, func(a, b int) int { if a < a { return -1 }; if a > b { return 1 }; return 0 })

	// A tagged switch is not the expressionless three-way switch.
	_ = slices.IsSortedFunc(xs, func(a, b int) int { switch true { case a < b: return -1; case a > b: return 1 }; return 0 })

	// A named comparator value is opaque — only a fresh literal matches.
	_ = slices.IsSortedFunc(xs, namedCmp)

	// SortedFunc on a different callee (the sort itself) is PS3013's
	// business, not PS3019's.
	slices.SortFunc(xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 })

	fmt.Println(xs, ys)
}

// A shadowed slices resolves IsSortedFunc to a METHOD of the local object,
// not the stdlib package function — never matched.
func shadowedSlices(xs []int) bool {
	slices := fakeSlices{}
	return slices.IsSortedFunc(xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 })
}
