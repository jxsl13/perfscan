package ps3015

import (
	sl "slices"
	st "strings"
)

// Both packages are resolved by IMPORT PATH, not spelling: an aliased slices
// keeps its alias in the rewrite, and the aliased strings spec is dropped
// when the rewrite deletes its last reference. A string ALIAS element
// (identical to string) is fixed like plain string.
type text = string

func sortedAliased(ys []text) []text {
	return sl.SortedFunc(sl.Values(ys), func(a, b text) int { return st.Compare(a, b) }) // want `slices\.SortedFunc with a bare strings\.Compare\(a, b\) comparator pays an indirect comparator call plus a strings\.Compare call per comparison; slices\.Sorted collects and sorts the (string|text) elements in the identical byte-lexicographic order with the comparison inlined`
}
