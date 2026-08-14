package ps3109

import (
	"cmp"
	"slices"
)

type rec struct{ id int }

// Swapped operands = descending order — never matched (that is a different sort).
func descending(xs []int, t int) (int, bool) {
	return slices.BinarySearchFunc(xs, t, func(a, b int) int { return cmp.Compare(b, a) })
}

// A field selector, not the bare parameters.
func byField(xs []rec, t rec) (int, bool) {
	return slices.BinarySearchFunc(xs, t, func(a, b rec) int { return cmp.Compare(a.id, b.id) })
}

// A named func value, not a fresh literal or cmp.Compare itself.
func myCmp(a, b int) int { return cmp.Compare(a, b) }

func named(xs []int, t int) (int, bool) {
	return slices.BinarySearchFunc(xs, t, myCmp)
}

// Already the direct call — nothing to do.
func direct(xs []int, t int) (int, bool) {
	return slices.BinarySearch(xs, t)
}

// An explicit instantiation: BinarySearchFunc's THREE type arguments do not
// transfer to BinarySearch's two — reported, but advisory (not auto-fixed).
func instantiated(xs []int, t int) (int, bool) {
	return slices.BinarySearchFunc[[]int, int, int](xs, t, cmp.Compare) // want `slices\.BinarySearchFunc with a bare cmp\.Compare comparator`
}
