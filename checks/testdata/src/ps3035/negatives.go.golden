package ps3035

import (
	"fmt"
	"math"
	"slices"
)

type item struct{ id int }

type fakeSlices struct{}

func (fakeSlices) MaxFunc(s []int, f func(a, b int) int) int { return 0 }

const negOne = -1

var namedRevCmp = func(a, b int) int { if a > b { return -1 }; if a < b { return 1 }; return 0 }

// None of these is the exact SWAPPED hand-rolled three-way; none is reported
// by PS3035, and none is rewritten.
func negatives(xs []int, ys []item, base int) {
	// The SOURCE-ORDER sign pair (a<b -> negative) is the natural order —
	// PS3022's case (MaxFunc -> Max), never PS3035's.
	slices.MaxFunc(xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 })

	// The reversed subtraction comparator can OVERFLOW (b-a for b=MinInt,
	// a=1) and is explicitly not matched.
	slices.MaxFunc(xs, func(a, b int) int { return b - a })

	// '>=' is not the three-way: it answers -1 for EQUAL pairs, an
	// inconsistent comparator.
	slices.MaxFunc(xs, func(a, b int) int { if a >= b { return -1 }; if a < b { return 1 }; return 0 })

	// A nonzero default is not the three-way.
	slices.MinFunc(xs, func(a, b int) int { if a > b { return -1 }; if a < b { return 1 }; return 1 })

	// SAME direction twice (b < a is a>b again): a<b falls through to 0 —
	// not a three-way, and not the reversed order.
	slices.MaxFunc(xs, func(a, b int) int { if a > b { return -1 }; if b < a { return 1 }; return 0 })

	// A NAMED constant (math.MinInt) instead of a literal: deleting the
	// comparator would orphan the math import, so the shape is not matched
	// even though the sign is right.
	slices.MaxFunc(xs, func(a, b int) int { if a > b { return math.MinInt }; if a < b { return 1 }; return 0 })

	// A local named constant is equally opaque to the literal-only shape.
	slices.MinFunc(xs, func(a, b int) int { if a > b { return negOne }; if a < b { return 1 }; return 0 })

	// A captured variable instead of the second parameter.
	slices.MaxFunc(xs, func(a, b int) int { if a > base { return -1 }; if a < base { return 1 }; return 0 })

	// Field selectors: the comparison is not on the bare parameters (a keyed
	// extremum keeps the whole struct; struct ties are observable).
	slices.MaxFunc(ys, func(a, b item) int { if a.id > b.id { return -1 }; if a.id < b.id { return 1 }; return 0 })

	// Extra work in the body: not the bare three-way.
	slices.MinFunc(xs, func(a, b int) int { fmt.Println(a, b); if a > b { return -1 }; if a < b { return 1 }; return 0 })

	// A repeated parameter (a > a) is not a comparison of the two.
	slices.MaxFunc(xs, func(a, b int) int { if a > a { return -1 }; if a < b { return 1 }; return 0 })

	// A tagged switch is not the expressionless three-way switch.
	slices.MaxFunc(xs, func(a, b int) int { switch true { case a > b: return -1; case a < b: return 1 }; return 0 })

	// A named comparator value is opaque — only a fresh literal matches (the
	// swapped cmp.Compare spelling is PS3020's, not PS3035's).
	slices.MaxFunc(xs, namedRevCmp)

	// SortFunc is a different callee; a descending sort has no Min/Max
	// counterpart and is unmatched here.
	slices.SortFunc(xs, func(a, b int) int { if a > b { return -1 }; if a < b { return 1 }; return 0 })

	fmt.Println(xs, ys)
}

// A shadowed slices resolves MaxFunc to a METHOD of the local object, not
// the stdlib package function — never matched.
func shadowedSlices(xs []int) {
	slices := fakeSlices{}
	slices.MaxFunc(xs, func(a, b int) int { if a > b { return -1 }; if a < b { return 1 }; return 0 })
	fmt.Println(xs)
}
