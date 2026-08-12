package ps3105

import (
	"fmt"
	"sort"
)

type fakeSorter struct{}

func (fakeSorter) Sort(x []int) {}

// A local variable shadows the package name: fakeSorter's Sort method is
// not the sort package function — never flagged.
func shadowedSort(xs []int) {
	sort := fakeSorter{}
	sort.Sort(xs)
}

// byLen is a custom sort.Interface implementation. sort.Sort(byLen(xs))
// IS a conversion call, but not to sort.IntSlice/StringSlice — never
// flagged (its ordering is not ascending-by-<).
type byLen []string

func (b byLen) Len() int           { return len(b) }
func (b byLen) Less(i, j int) bool { return len(b[i]) < len(b[j]) }
func (b byLen) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func customInterface(xs []string) {
	sort.Sort(byLen(xs))
}

// sort.Stable over the same custom sort.Interface is likewise not
// ascending-by-< — never flagged.
func customInterfaceStable(xs []string) {
	sort.Stable(byLen(xs))
}

// NEVER flagged: float64 has distinguishable ties (-0.0/+0.0, NaN
// payloads) that an unstable sort may arrange differently, and
// Float64Slice.Less orders NaNs first — the rewrite is not guaranteed
// bit-identical, so PS3105 excludes it.
func floats(fs []float64) {
	sort.Sort(sort.Float64Slice(fs))
}

// Also excluded under sort.Stable: the source is stable but slices.Sort
// is not, so float64's distinguishable ties could be reordered — not
// guaranteed bit-identical.
func floatsStable(fs []float64) {
	sort.Stable(sort.Float64Slice(fs))
}

// The helper spelling sort.Ints(x) belongs to PS3104, not PS3105.
func helperSpelling(xs []int) {
	sort.Ints(xs)
}

// A PRE-BUILT sort.IntSlice value is not a fresh conversion: the
// underlying operand is not visible in the call, so PS3105 never matches
// it — under sort.Sort or sort.Stable.
func prebuilt(xs []int) {
	var p sort.IntSlice = xs
	sort.Sort(p)
	sort.Stable(p)
}

// Only a plain call STATEMENT is rewritten; go/defer are left alone —
// for both spellings.
func deferred(xs []int) {
	defer sort.Sort(sort.IntSlice(xs))
	defer sort.Stable(sort.IntSlice(xs))
	fmt.Println(len(xs))
}

// An untyped nil operand is never flagged: sort.Sort(sort.IntSlice(nil))
// is legal, but slices.Sort(nil) cannot infer its type parameters and
// would not compile. Same for the stable spelling.
func untypedNil() {
	sort.Sort(sort.IntSlice(nil))
	sort.Stable(sort.IntSlice(nil))
}

// Reported but NOT fixed: a comment inside the replaced call punctuation
// would be destroyed by the rewrite.
func commented(xs []int) {
	sort.Sort( /* keep me */ sort.IntSlice(xs)) // want `sort\.Sort\(sort\.IntSlice\(\.\.\.\)\) sorts through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.Sort sorts the concrete \[\]int directly with the identical ascending order`
}
