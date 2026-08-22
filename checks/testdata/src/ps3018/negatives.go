package ps3018

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

type fakeStrings struct{}

func (fakeStrings) Compare(a, b string) int { return len(b) - len(a) }

type name string

var cmpFn = func(a, b string) int { return strings.Compare(a, b) }

// None of these is the bare ascending strings.Compare comparator on
// slices.SortedStableFunc; none is reported, and none is rewritten.
func negatives(ys []string, xs []int, base string) {
	// Swapped operands: strings.Compare(b, a) is a DESCENDING sort — never
	// slices.Sorted.
	fmt.Println(slices.SortedStableFunc(slices.Values(ys), func(a, b string) int { return strings.Compare(b, a) }))

	// Extra work in the body: not a bare comparator.
	fmt.Println(slices.SortedStableFunc(slices.Values(ys), func(a, b string) int {
		fmt.Println(a, b)
		return strings.Compare(a, b)
	}))

	// A captured variable instead of the second parameter.
	fmt.Println(slices.SortedStableFunc(slices.Values(ys), func(a, b string) int { return strings.Compare(a, base) }))

	// Arithmetic around the compare: not the bare call.
	fmt.Println(slices.SortedStableFunc(slices.Values(ys), func(a, b string) int { return -strings.Compare(a, b) }))

	// A DEFINED string type needs conversions to reach strings.Compare —
	// the arguments are no longer the bare parameters, so the exact match
	// fails (the ordering would still agree, but the matcher stays exact).
	ns := []name{"b", "a"}
	fmt.Println(slices.SortedStableFunc(slices.Values(ns), func(a, b name) int { return strings.Compare(string(a), string(b)) }))

	// cmp.Compare is a different package's comparator; PS3016 owns that
	// spelling.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { return cmp.Compare(a, b) }))

	// A named comparator value is opaque — only a fresh literal or
	// strings.Compare itself matches.
	fmt.Println(slices.SortedStableFunc(slices.Values(ys), cmpFn))

	// SortedFunc is a different callee (PS3015's, the unstable collecting
	// sort); unmatched here.
	fmt.Println(slices.SortedFunc(slices.Values(ys), func(a, b string) int { return strings.Compare(a, b) }))

	// slices.SortStableFunc is the eager sibling — a different family owns
	// that callee.
	slices.SortStableFunc(ys, func(a, b string) int { return strings.Compare(a, b) })

	// Already the direct call.
	fmt.Println(slices.Sorted(slices.Values(ys)))
}

// A shadowed strings resolves the Compare selector to a METHOD of the local
// object, not the stdlib package function — never matched.
func shadowedStrings(ys []string) []string {
	strings := fakeStrings{}
	return slices.SortedStableFunc(slices.Values(ys), func(a, b string) int { return strings.Compare(a, b) })
}
