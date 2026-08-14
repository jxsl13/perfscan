package ps3009cgo

// The only strings reference in this cgo FILE is the fixable comparator
// itself: the slices.Sort rewrite would orphan the import, and a cgo
// file's import block is never pruned, so the fix is withheld — the
// report stays advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import (
	"slices"
	"strings"
)

func cgoSort(ys []string) {
	slices.SortFunc(ys, func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.Sort sorts the string elements in the identical byte-lexicographic order with the comparison inlined`
}
