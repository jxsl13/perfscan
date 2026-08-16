package ps3032

import (
	"fmt"
	"sort"
)

// The predicate is rewritten wherever its bool is consumed — condition,
// assignment, return. These calls are the file's ONLY sort references
// (two per call: sort.IsSorted plus the conversion), so the fix also
// swaps the orphaned sort import for slices.
func checkBoth(xs []int, ss []string) bool {
	if sort.IsSorted(sort.IntSlice(xs)) { // want `sort\.IsSorted\(sort\.IntSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
		fmt.Println("ints sorted")
	}
	ok := sort.IsSorted(sort.StringSlice(ss)) // want `sort\.IsSorted\(sort\.StringSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]string directly with the identical boolean result`
	return ok && sort.IsSorted(sort.IntSlice(xs)) // want `sort\.IsSorted\(sort\.IntSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
}

// The operand expression is kept verbatim, however it is spelled.
func checkField(w struct{ ids []int }) bool {
	return sort.IsSorted(sort.IntSlice(w.ids)) // want `sort\.IsSorted\(sort\.IntSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
}

type intList []int

// A NAMED slice type is fine: the conversion sort.IntSlice(xs) accepts it
// (same underlying []int), and slices.IsSorted infers the element through
// ~[]E and compares the ordered underlying int values — never a method —
// so the bool is unchanged.
func checkNamed(xs intList) bool {
	return sort.IsSorted(sort.IntSlice(xs)) // want `sort\.IsSorted\(sort\.IntSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
}

// A call ARGUMENT position: both spellings are primary expressions, so no
// parenthesization is ever needed around the replacement.
func checkArg(xs []int) {
	fmt.Println(sort.IsSorted(sort.IntSlice(xs))) // want `sort\.IsSorted\(sort\.IntSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
}
