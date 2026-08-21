package ps3024

import (
	"fmt"
	"sort"
)

// Every call in this file has the fixable shape — a pure-path slice and the
// whole-element ascending closure — and they are the file's ONLY sort
// references, so the fix also swaps the orphaned sort import for "slices".

// The predicate is rewritten wherever its bool is consumed — condition,
// assignment, return.
func checkInts(xs []int) bool {
	return sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] < xs[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

func checkStrings(ss []string) {
	if sort.SliceIsSorted(ss, func(i, j int) bool { return ss[i] < ss[j] }) { // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
		fmt.Println("sorted")
	}
}

type intList []int

// A NAMED slice type is fine: slices.IsSorted infers the element through
// ~[]E and compares the ordered underlying int values — never a method —
// so the bool is unchanged.
func checkNamed(xs intList) bool {
	return sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] < xs[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

type holder struct{ ids []uint64 }

// The tested value is a selector chain (h.ids) — a side-effect-free path,
// evaluated once as the call argument in both spellings — so it is FIXED,
// keeping the original path spelling verbatim in place.
func checkField(h *holder) bool {
	ok := sort.SliceIsSorted(h.ids, func(i, j int) bool { return h.ids[i] < h.ids[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
	return ok
}
