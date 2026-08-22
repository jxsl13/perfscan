package ps3107

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

type fakeCmp struct{}

func (fakeCmp) Compare(a, b int) int { return b - a }

var cmpFn = func(a, b int) int { return cmp.Compare(a, b) }

// None of these is the bare ascending cmp.Compare comparator; none is
// reported, and none is rewritten.
func negatives(xs []int, ys []string, base int) {
	// Swapped operands: cmp.Compare(b, a) is a DESCENDING sort — exactly
	// what PS3105's Reverse fix produces — never slices.Sort.
	slices.SortFunc(xs, func(a, b int) int { return cmp.Compare(b, a) })

	// Extra work in the body: not a bare comparator.
	slices.SortFunc(xs, func(a, b int) int {
		fmt.Println(a, b)
		return cmp.Compare(a, b)
	})

	// A captured variable instead of the second parameter.
	slices.SortFunc(xs, func(a, b int) int { return cmp.Compare(a, base) })

	// Arithmetic around the compare: not the bare call.
	slices.SortFunc(xs, func(a, b int) int { return -cmp.Compare(a, b) })

	// strings.Compare is a different package's comparator; out of scope.
	slices.SortFunc(ys, func(a, b string) int { return strings.Compare(a, b) })

	// A named comparator value is opaque — only a fresh literal or
	// cmp.Compare itself matches.
	slices.SortFunc(xs, cmpFn)

	// SortStableFunc is a different callee (the stable sort); unmatched.
	slices.SortStableFunc(xs, func(a, b int) int { return cmp.Compare(a, b) })

	fmt.Println(xs, ys)
}

// A shadowed cmp resolves the Compare selector to a METHOD of the local
// object, not the stdlib package function — never matched.
func shadowedCmp(xs []int) {
	cmp := fakeCmp{}
	slices.SortFunc(xs, func(a, b int) int { return cmp.Compare(a, b) })
	fmt.Println(xs)
}
