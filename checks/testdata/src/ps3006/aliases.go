package ps3006

import (
	c "cmp"
	"fmt"
	sl "slices"
)

// Both packages are resolved by IMPORT PATH, not spelling: an aliased
// slices keeps its alias in the rewrite, and the aliased cmp spec is
// dropped when the rewrite deletes its last reference.
func sortAliased(xs []int) {
	sl.SortStableFunc(xs, func(a, b int) int { return c.Compare(a, b) }) // want `slices\.SortStableFunc with a bare cmp\.Compare\(a, b\) comparator pays an indirect comparator call per comparison plus the stable sort's merge overhead; slices\.Sort sorts the int elements with the identical ascending order, the comparison inlined and no stability cost`
	fmt.Println(xs)
}
