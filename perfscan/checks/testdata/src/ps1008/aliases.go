package ps1008

import (
	sl "slices"
)

// An aliased slices import matches through type info and the fix keeps the
// alias as the qualifier.
func eqAliased(a, b []int) bool {
	return sl.EqualFunc(a, b, func(x, y int) bool { return x == y }) // want `slices\.Equal compares the int elements with the identical result and the comparison inlined`
}
