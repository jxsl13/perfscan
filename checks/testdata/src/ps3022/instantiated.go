package ps3022

import (
	"fmt"
	"slices"
)

// Explicit instantiation brackets SURVIVE the rewrite: slices.Max/Min take
// the same two type parameters as slices.MaxFunc/MinFunc (and every fixable
// element type satisfies their cmp.Ordered constraint), so only the selector
// name changes and the comparator goes.
func maxInstantiated(xs []int) {
	m := slices.MaxFunc[[]int, int](xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 }) // want `slices\.Max computes the extremal int value with the identical result and the comparison inlined`
	n := slices.MinFunc[[]int](xs, func(a, b int) int { if a < b { return -1 }; if a > b { return 1 }; return 0 })      // want `slices\.Min computes the extremal int value with the identical result and the comparison inlined`
	fmt.Println(m, n)
}
