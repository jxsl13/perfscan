package ps3006

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

type fakeCmp struct{}

func (fakeCmp) Compare(a, b int) int { return b - a }

var cmpFn = func(a, b int) int { return cmp.Compare(a, b) }

// None of these is the bare ascending cmp.Compare comparator under
// SortStableFunc; none is reported, and none is rewritten.
func negatives(xs []int, ys []string, base int) {
	// Swapped operands: cmp.Compare(b, a) is a DESCENDING sort — never
	// slices.Sort.
	slices.SortStableFunc(xs, func(a, b int) int { return cmp.Compare(b, a) })

	// Extra work in the body: not a bare comparator.
	slices.SortStableFunc(xs, func(a, b int) int {
		fmt.Println(a, b)
		return cmp.Compare(a, b)
	})

	// A captured variable instead of the second parameter.
	slices.SortStableFunc(xs, func(a, b int) int { return cmp.Compare(a, base) })

	// Arithmetic around the compare: not the bare call.
	slices.SortStableFunc(xs, func(a, b int) int { return -cmp.Compare(a, b) })

	// strings.Compare is a different package's comparator; out of scope.
	slices.SortStableFunc(ys, func(a, b string) int { return strings.Compare(a, b) })

	// A named comparator value is opaque — only a fresh literal or
	// cmp.Compare itself matches.
	slices.SortStableFunc(xs, cmpFn)

	// SortFunc is a different callee (the unstable sort); that spelling
	// is PS3107's, not PS3006's.
	slices.SortFunc(xs, func(a, b int) int { return cmp.Compare(a, b) })

	fmt.Println(xs, ys)
}

// A shadowed cmp resolves the Compare selector to a METHOD of the local
// object, not the stdlib package function — never matched.
func shadowedCmp(xs []int) {
	cmp := fakeCmp{}
	slices.SortStableFunc(xs, func(a, b int) int { return cmp.Compare(a, b) })
	fmt.Println(xs)
}
