package ps3014

import (
	sl "slices"
	st "strings"
)

// Both packages are resolved by IMPORT PATH, not spelling: an aliased slices
// keeps its alias in the rewrite, and the aliased strings spec is dropped
// when the rewrite deletes its last reference. A string ALIAS element
// (identical to string) is fixed like plain string.
type text = string

func sortedAliased(ys []text) bool {
	return sl.IsSortedFunc(ys, func(a, b text) int { return st.Compare(a, b) }) // want `slices\.IsSortedFunc with a bare strings\.Compare comparator`
}
