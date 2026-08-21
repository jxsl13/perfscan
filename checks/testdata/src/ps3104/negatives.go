package ps3104

import (
	"fmt"
	"sort"
)

type fakeSorter struct{}

func (fakeSorter) Ints([]int) {}

// A local variable shadows the package name: fakeSorter's Ints method is
// not the sort package function — never flagged.
func shadowedSort(xs []int) {
	sort := fakeSorter{}
	sort.Ints(xs)
}

// The helper stored as a value is not a call statement on the package
// selector — never flagged (and the reference keeps the import alive).
func storedValue(xs []int) {
	f := sort.Ints
	f(xs)
}

// Only a plain call STATEMENT is rewritten; go/defer are left alone.
func deferred(xs []int) {
	defer sort.Ints(xs)
	fmt.Println(len(xs))
}

// Reported but NOT fixed: a comment inside the replaced call punctuation
// would be destroyed by the rewrite.
func commented(xs []int) {
	sort.Ints( /* keep me */ xs) // want `sort\.Ints is the legacy spelling of slices\.Sort \(an interface-dispatch sort on go1\.21, a one-line wrapper since go1\.22\); slices\.Sort sorts the concrete \[\]int directly with the identical ascending order`
}
