package ps3020

import (
	"cmp"
	"slices"
)

type Priority int

// The swapped cmp.Compare(b, a) comparator reverses the order, so MaxFunc
// selects the MINIMUM and MinFunc the MAXIMUM. Every site here is an integer
// element and is fixed to the opposite direct extremum; the rewrites remove
// the file's last cmp references, so the cmp import is dropped too. Slice
// expressions kept verbatim.

// MaxFunc under the reversed order is the minimum.
func lowest(xs []int) int {
	return slices.MaxFunc(xs, func(a, b int) int { return cmp.Compare(b, a) }) // want `slices\.MaxFunc with a swapped cmp\.Compare\(b, a\) comparator selects the minimum`
}

// MinFunc under the reversed order is the maximum; a named ordered element is
// fixed too.
func highest(ps []Priority) Priority {
	return slices.MinFunc(ps, func(a, b Priority) int { return cmp.Compare(b, a) }) // want `slices\.MinFunc with a swapped cmp\.Compare\(b, a\) comparator selects the maximum`
}

// Split parameter fields (func(a T, b T)) match the same way.
func lowest8(xs []uint8) uint8 {
	return slices.MaxFunc(xs, func(a uint8, b uint8) int { return cmp.Compare(b, a) }) // want `slices\.MaxFunc with a swapped cmp\.Compare\(b, a\) comparator selects the minimum`
}
