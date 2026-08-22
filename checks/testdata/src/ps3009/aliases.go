package ps3009

import (
	"fmt"
	sl "slices"
	st "strings"
)

// Both packages are resolved by IMPORT PATH, not spelling: an aliased
// slices keeps its alias in the rewrite, and the aliased strings spec is
// dropped when the rewrite deletes its last reference. A string ALIAS
// element (identical to string) is fixed like plain string.
type text = string

func sortAliased(ys []text) {
	sl.SortFunc(ys, func(a, b text) int { return st.Compare(a, b) }) // want `slices\.Sort sorts the (string|text) elements in the identical byte-lexicographic order with the comparison inlined`
	fmt.Println(ys)
}
