package ps3022

import (
	"fmt"
	sl "slices"
)

// The package is resolved by IMPORT PATH, not spelling: an aliased slices
// keeps its alias in the rewrite. The comparator references nothing but its
// own parameters and literals, so no import is ever orphaned.
func maxAliased(xs []int) {
	m := sl.MaxFunc(xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.MaxFunc with a hand-rolled three-way comparator \(a<b/a>b/-1/1/0\) pays an indirect comparator call plus up to two relational comparisons per element; slices\.Max computes the extremal int value with the identical result and the comparison inlined`
	fmt.Println(m)
}
