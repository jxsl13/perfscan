package ps3027

import (
	sl "slices"
)

// An aliased slices import: the fix keeps the file's qualifier.
func compareAliased(a, b []int) int {
	return sl.CompareFunc(a, b, func(x, y int) int { if x < y { return -1 }; if x > y { return 1 }; return 0 }) // want `slices\.Compare compares the int elements with the identical lexicographic result and the comparison inlined`
}

// A parenthesized comparator still matches and rewrites.
func compareParens(a, b []uint16) int {
	return sl.CompareFunc(a, b, (func(x, y uint16) int { if x < y { return -1 }; if x > y { return 1 }; return 0 })) // want `slices\.Compare compares the uint16 elements with the identical lexicographic result and the comparison inlined`
}
