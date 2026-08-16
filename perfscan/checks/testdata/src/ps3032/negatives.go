package ps3032

import (
	"fmt"
	"sort"
)

type fakeChecker struct{}

func (fakeChecker) IsSorted(x []int) bool { return false }

// A local variable shadows the package name: fakeChecker's IsSorted method
// is not the sort package function — never flagged.
func shadowedSort(xs []int) bool {
	sort := fakeChecker{}
	return sort.IsSorted(xs)
}

// The predicate stored as a value is not a call on the package selector —
// never flagged (and the reference keeps the import alive).
func storedValue(xs []int) bool {
	f := sort.IsSorted
	return f(sort.IntSlice(xs))
}

// byLen is a custom sort.Interface implementation. sort.IsSorted(byLen(xs))
// IS a conversion call, but not to sort.IntSlice/StringSlice — never
// flagged (its ordering is not ascending-by-<).
type byLen []string

func (b byLen) Len() int           { return len(b) }
func (b byLen) Less(i, j int) bool { return len(b[i]) < len(b[j]) }
func (b byLen) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func customInterface(xs []string) bool {
	return sort.IsSorted(byLen(xs))
}

// NEVER flagged: the sort family keeps a bright no-float line —
// Float64Slice.Less has NaN-ordering semantics, and perfscan does not
// special-case float comparisons even where a rewrite could be argued.
func floats(fs []float64) bool {
	return sort.IsSorted(sort.Float64Slice(fs))
}

// NEVER flagged: the DESCENDING spelling through sort.Reverse has no
// comparator-free slices target (slices has no descending IsSorted), so
// PS3032 deliberately leaves it alone.
func descending(xs []int) bool {
	return sort.IsSorted(sort.Reverse(sort.IntSlice(xs)))
}

// A PRE-BUILT sort.IntSlice value is not a fresh conversion: the
// underlying operand is not visible in the call, so PS3032 never matches
// it.
func prebuilt(xs []int) bool {
	var p sort.IntSlice = xs
	return sort.IsSorted(p)
}

// An untyped nil operand is never flagged: sort.IsSorted(sort.IntSlice(nil))
// is legal, but slices.IsSorted(nil) cannot infer its type parameters and
// would not compile.
func untypedNil() bool {
	return sort.IsSorted(sort.IntSlice(nil))
}

// The helper spelling sort.IntsAreSorted(x) belongs to PS3008, not PS3032.
func helperSpelling(xs []int) bool {
	return sort.IntsAreSorted(xs)
}

// Reported but NOT fixed: a comment inside the replaced call punctuation
// would be destroyed by the rewrite.
func commented(xs []int) {
	fmt.Println(sort.IsSorted( /* keep me */ sort.IntSlice(xs))) // want `sort\.IsSorted\(sort\.IntSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
}

// Reported but NOT fixed: a local variable shadows the slices identifier at
// the call site, so the rewritten qualifier would resolve to it.
func shadowedSlices(xs []int) bool {
	slices := 0
	_ = slices
	return sort.IsSorted(sort.IntSlice(xs)) // want `sort\.IsSorted\(sort\.IntSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
}
