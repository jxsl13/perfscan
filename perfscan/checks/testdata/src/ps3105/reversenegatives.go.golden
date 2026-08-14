package ps3105

import (
	"fmt"
	"sort"
)

// NEVER flagged, Reverse form included: float64 has distinguishable ties
// (-0.0/+0.0, NaN payloads) and Float64Slice.Less orders NaNs first — the
// descending rewrite is not guaranteed bit-identical either, so PS3105
// excludes the Reverse form of Float64Slice too, under sort.Sort and
// sort.Stable alike.
func reverseFloats(fs []float64) {
	sort.Sort(sort.Reverse(sort.Float64Slice(fs)))
	sort.Stable(sort.Reverse(sort.Float64Slice(fs)))
}

// A PRE-BUILT sort.IntSlice value handed to sort.Reverse is not a fresh
// conversion: the underlying operand is not visible in the call, so the
// Reverse form is never matched either.
func reversePrebuilt(xs []int) {
	var p sort.IntSlice = xs
	sort.Sort(sort.Reverse(p))
	sort.Stable(sort.Reverse(p))
}

// A double Reverse is ascending again, but sort.Reverse's argument is
// another sort.Reverse call, not a fresh adapter conversion — never
// matched.
func reverseTwice(xs []int) {
	sort.Sort(sort.Reverse(sort.Reverse(sort.IntSlice(xs))))
}

// An untyped nil operand is never flagged in the Reverse form either:
// slices.SortFunc(nil, ...) cannot infer its slice type parameter from
// nil and would not compile.
func reverseUntypedNil() {
	sort.Sort(sort.Reverse(sort.IntSlice(nil)))
}

// Only a plain call STATEMENT is rewritten; go/defer are left alone.
func reverseDeferred(xs []int) {
	defer sort.Sort(sort.Reverse(sort.IntSlice(xs)))
	fmt.Println(len(xs))
}

type fakeRev struct{}

func (fakeRev) Sort(x any)             {}
func (fakeRev) Reverse(x []int) []int  { return x }
func (fakeRev) IntSlice(x []int) []int { return x }

// A local variable shadows the package name through the WHOLE chain:
// fakeRev's Sort/Reverse/IntSlice methods are not the sort package's
// objects — never flagged.
func shadowedReverse(xs []int) {
	sort := fakeRev{}
	sort.Sort(sort.Reverse(sort.IntSlice(xs)))
}

// Reported but NOT fixed: a comment inside the replaced call punctuation
// would be destroyed by the rewrite — same guard as the ascending form.
func reverseCommented(xs []int) {
	sort.Sort(sort.Reverse( /* keep me */ sort.IntSlice(xs))) // want `sort\.Sort\(sort\.Reverse\(sort\.IntSlice\(\.\.\.\)\)\) sorts descending through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.SortFunc with cmp\.Compare\(b, a\) sorts the concrete \[\]int directly with the identical descending order`
}
