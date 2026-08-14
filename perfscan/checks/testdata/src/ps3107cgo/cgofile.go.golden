package ps3107cgo

// The only cmp reference in this cgo FILE is the fixable comparator
// itself: the slices.Sort rewrite would orphan the import, and a cgo
// file's import block is never pruned, so the fix is withheld — the
// report stays advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import (
	"cmp"
	"slices"
)

func cgoSort(xs []int) {
	slices.SortFunc(xs, func(a, b int) int { return cmp.Compare(a, b) }) // want `slices\.Sort sorts the int elements with the identical ascending order and the comparison inlined`
}
