package ps3028

import (
	"fmt"
	"math"
	"slices"
)

type rec struct{ id int }

type fakeSlices struct{}

func (fakeSlices) BinarySearchFunc(s []int, t int, f func(a, b int) int) (int, bool) { return 0, false }

const negOne = -1

var namedCmp = func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }

// None of these is the exact ascending hand-rolled three-way; none is
// reported, and none is rewritten.
func negatives(xs []int, ys []rec, t int, base int) {
	// The subtraction comparator can OVERFLOW (a-b for a=MinInt, b=1) and is
	// explicitly not matched.
	slices.BinarySearchFunc(xs, t, func(a, b int) int { return a - b })

	// Signs swapped: a<b returning POSITIVE is a DESCENDING search — never
	// slices.BinarySearch.
	slices.BinarySearchFunc(xs, t, func(a, b int) int { if a < b { return 1 }; if a > b { return -1 }; return 0 })

	// '<=' is not the three-way: it answers -1 for EQUAL pairs, so the found
	// check could never fire.
	slices.BinarySearchFunc(xs, t, func(a, b int) int { if a <= b { return -1 }; if a > b { return 1 }; return 0 })

	// A nonzero default is not the three-way.
	slices.BinarySearchFunc(xs, t, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 1 })

	// SAME direction twice (b > a is a<b again): a>b falls through to 0 — not
	// a three-way, and not the search's order.
	slices.BinarySearchFunc(xs, t, func(a, b int) int { if a < b { return -1 }; if b > a { return 1 }; return 0 })

	// A NAMED constant (math.MinInt) instead of a literal: deleting the
	// comparator would orphan the math import, so the shape is not matched
	// even though the sign is right.
	slices.BinarySearchFunc(xs, t, func(a, b int) int { if a < b { return math.MinInt }; if a > b { return 1 }; return 0 })

	// A local named constant is equally opaque to the literal-only shape.
	slices.BinarySearchFunc(xs, t, func(a, b int) int { if a < b { return negOne }; if a > b { return 1 }; return 0 })

	// A captured variable instead of the second parameter.
	slices.BinarySearchFunc(xs, t, func(a, b int) int { if a < base { return -1 }; if a > base { return 1 }; return 0 })

	// Field selectors: the comparison is not on the bare parameters, and a
	// struct element is not cmp.Ordered anyway.
	slices.BinarySearchFunc(ys, rec{id: t}, func(a, b rec) int { if a.id < b.id { return -1 }; if a.id > b.id { return 1 }; return 0 })

	// Extra work in the body: not the bare three-way (the side effect would
	// be lost).
	slices.BinarySearchFunc(xs, t, func(a, b int) int { fmt.Println(a, b); if a < b { return -1 }; if a > b { return 1 }; return 0 })

	// A repeated parameter (a < a) is not a comparison of the two.
	slices.BinarySearchFunc(xs, t, func(a, b int) int { if a < a { return -1 }; if a > b { return 1 }; return 0 })

	// A tagged switch is not the expressionless three-way switch.
	slices.BinarySearchFunc(xs, t, func(a, b int) int { switch true { case a < b: return -1; case a > b: return 1 }; return 0 })

	// A named comparator value is opaque — only a fresh literal matches.
	slices.BinarySearchFunc(xs, t, namedCmp)

	// Already the direct call — nothing to do.
	slices.BinarySearch(xs, t)

	fmt.Println(xs, ys)
}

// A shadowed slices resolves BinarySearchFunc to a METHOD of the local
// object, not the stdlib package function — never matched.
func shadowedSlices(xs []int, t int) {
	slices := fakeSlices{}
	slices.BinarySearchFunc(xs, t, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 })
	fmt.Println(xs)
}
