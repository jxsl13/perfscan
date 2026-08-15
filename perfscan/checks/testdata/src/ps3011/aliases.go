package ps3011

import (
	sl "slices"
	st "strings"
)

// Both packages are resolved by IMPORT PATH, not spelling: an aliased slices
// keeps its alias in the rewrite, and the aliased strings spec is dropped when
// the rewrite deletes its last reference. A string ALIAS element (identical to
// string) is fixed like plain string.
type text = string

func searchAliased(ys []text, t text) (int, bool) {
	return sl.BinarySearchFunc(ys, t, func(a, b text) int { return st.Compare(a, b) }) // want `slices\.BinarySearchFunc with a bare strings\.Compare comparator`
}
