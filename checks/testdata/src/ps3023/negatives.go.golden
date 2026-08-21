package ps3023

import (
	"cmp"
	"slices"
)

type IntSlice []int

// MIXED SLICE TYPES: advisory only (reported, NOT fixed) — CompareFunc takes
// two independently-typed slices, slices.Compare one type S for both, so the
// rewrite would not typecheck.
func mixed(a []int, b IntSlice) int {
	return slices.CompareFunc(a, b, cmp.Compare) // want `slices\.CompareFunc with a bare cmp\.Compare comparator`
}

// EXPLICIT INSTANTIATION: advisory only — CompareFunc has four type
// parameters, Compare two, so the brackets cannot survive the rewrite.
func instantiated(a, b []int) int {
	return slices.CompareFunc[[]int, []int, int, int](a, b, cmp.Compare) // want `slices\.CompareFunc with a bare cmp\.Compare comparator`
}

// A comment inside the deleted span would be destroyed: advisory only.
func commented(a, b []int) int {
	return slices.CompareFunc(a, b, cmp.Compare /* three-way */) // want `slices\.CompareFunc with a bare cmp\.Compare comparator`
}

// Swapped operands = slices.Compare(b, a), a DIFFERENT result — never matched.
func swapped(a, b []int) int {
	return slices.CompareFunc(a, b, func(x, y int) int { return cmp.Compare(y, x) })
}

// A custom comparator, not cmp.Compare.
func custom(a, b []int) int {
	return slices.CompareFunc(a, b, func(x, y int) int {
		if x > y {
			return 1
		}
		return -1
	})
}

// The comparator held in a variable is not a fresh literal / cmp.Compare
// itself — never matched.
func viaVariable(a, b []int) int {
	c := cmp.Compare[int]
	return slices.CompareFunc(a, b, c)
}

// Already the direct call.
func direct(a, b []int) int {
	return slices.Compare(a, b)
}
