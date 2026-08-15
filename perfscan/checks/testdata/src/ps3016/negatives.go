package ps3016

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

type fakeCmp struct{}

func (fakeCmp) Compare(a, b int) int { return b - a }

var cmpFn = func(a, b int) int { return cmp.Compare(a, b) }

// FLOAT elements: the ORDER is identical (cmp.Compare and slices.Sort both
// put NaNs first), but -0.0/+0.0 and distinct NaN payloads are
// equal-but-distinguishable ties that SortedStableFunc contractually keeps
// in collection order while the unstable rewrite may arrange either way, so
// byte-identity is not contractual — advisory, no fix.
func sortedFloats(fs []float64) []float64 {
	return slices.SortedStableFunc(slices.Values(fs), cmp.Compare) // want `slices\.Sorted collects and sorts the float64 elements with the identical ascending order, the comparison inlined and no stability cost \(no auto-fix: float ties -0\.0/\+0\.0 and NaN payloads are equal-but-distinguishable under an unstable sort\)`
}

// A TYPE-PARAMETER element may be instantiated with floats — same
// distinguishable-tie hazard — advisory, no fix.
func sortedAny[T cmp.Ordered](xs []T) []T {
	return slices.SortedStableFunc(slices.Values(xs), func(a, b T) int { return cmp.Compare(a, b) }) // want `slices\.Sorted collects and sorts the T elements with the identical ascending order, the comparison inlined and no stability cost \(no auto-fix: type-parameter element, instantiations may include floats whose ties are equal-but-distinguishable\)`
}

// A comment inside the span the rewrite would delete (after the seq
// argument) would be silently destroyed — advisory, no fix.
func sortedCommented(xs []int) []int {
	return slices.SortedStableFunc(slices.Values(xs), /* keep me */ cmp.Compare) // want `slices\.Sorted collects and sorts the int elements`
}

// None of these is the bare ascending cmp.Compare comparator; none is
// reported, and none is rewritten.
func negatives(xs []int, ys []string, base int) {
	// Swapped operands: cmp.Compare(b, a) is a DESCENDING sort — never
	// slices.Sorted.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { return cmp.Compare(b, a) }))

	// Extra work in the body: not a bare comparator.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int {
		fmt.Println(a, b)
		return cmp.Compare(a, b)
	}))

	// A captured variable instead of the second parameter.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { return cmp.Compare(a, base) }))

	// Arithmetic around the compare: not the bare call.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { return -cmp.Compare(a, b) }))

	// strings.Compare is a different package's comparator; out of scope.
	fmt.Println(slices.SortedStableFunc(slices.Values(ys), func(a, b string) int { return strings.Compare(a, b) }))

	// A named comparator value is opaque — only a fresh literal or
	// cmp.Compare itself matches.
	fmt.Println(slices.SortedStableFunc(slices.Values(xs), cmpFn))

	// SortedFunc is a different callee (PS3012's, the unstable collecting
	// sort); unmatched here.
	fmt.Println(slices.SortedFunc(slices.Values(xs), func(a, b int) int { return cmp.Compare(a, b) }))

	// Already the direct call.
	fmt.Println(slices.Sorted(slices.Values(xs)))
}

// A shadowed cmp resolves the Compare selector to a METHOD of the local
// object, not the stdlib package function — never matched.
func shadowedCmp(xs []int) []int {
	cmp := fakeCmp{}
	return slices.SortedStableFunc(slices.Values(xs), func(a, b int) int { return cmp.Compare(a, b) })
}
