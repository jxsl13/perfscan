package ps3107

import (
	c "cmp"
	"fmt"
	sl "slices"
)

// Both packages are resolved by IMPORT PATH, not spelling: an aliased
// slices keeps its alias in the rewrite, and the aliased cmp spec is
// dropped when the rewrite deletes its last reference.
func sortAliased(xs []int) {
	sl.SortFunc(xs, func(a, b int) int { return c.Compare(a, b) }) // want `slices\.SortFunc with a bare cmp\.Compare\(a, b\) comparator pays an indirect comparator call per comparison; slices\.Sort sorts the int elements with the identical ascending order and the comparison inlined`
	fmt.Println(xs)
}
