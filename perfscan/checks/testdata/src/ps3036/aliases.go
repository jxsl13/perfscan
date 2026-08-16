package ps3036

import (
	sl "slices"
	st "strings"
)

// Both packages are resolved by IMPORT PATH, not spelling: an aliased slices
// keeps its alias in the rewrite, and the aliased strings spec is dropped when
// the rewrite deletes its last reference. A string ALIAS element (identical to
// string) is fixed like plain string.
type text = string

func aliased(a, b []text) int {
	return sl.CompareFunc(a, b, func(x, y text) int { return st.Compare(x, y) }) // want `slices\.CompareFunc with a bare strings\.Compare comparator`
}
