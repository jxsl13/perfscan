package ps3008

import (
	"fmt"
	"sort"
)

type fakeChecker struct{}

func (fakeChecker) IntsAreSorted([]int) bool { return false }

// A local variable shadows the package name: fakeChecker's IntsAreSorted
// method is not the sort package function — never flagged.
func shadowedSort(xs []int) bool {
	sort := fakeChecker{}
	return sort.IntsAreSorted(xs)
}

// The predicate stored as a value is not a call on the package selector —
// never flagged (and the reference keeps the import alive).
func storedValue(xs []int) bool {
	f := sort.IntsAreSorted
	return f(xs)
}

// Reported but NOT fixed: sort.IntsAreSorted(nil) compiles (nil converts
// to the []int parameter), but the generic slices.IsSorted(nil) cannot
// infer its type parameters — the rewrite would not compile.
func nilArg() bool {
	return sort.IntsAreSorted(nil) // want `sort\.IntsAreSorted is the legacy spelling of slices\.IsSorted \(an interface-dispatch scan on go1\.21, a one-line wrapper since go1\.22\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
}

// Reported but NOT fixed: a comment inside the replaced call punctuation
// would be destroyed by the rewrite.
func commented(xs []int) {
	fmt.Println(sort.IntsAreSorted( /* keep me */ xs)) // want `sort\.IntsAreSorted is the legacy spelling of slices\.IsSorted \(an interface-dispatch scan on go1\.21, a one-line wrapper since go1\.22\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
}
