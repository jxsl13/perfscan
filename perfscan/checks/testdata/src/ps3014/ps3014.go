package ps3014

import (
	"slices"
	"strings"
)

// The bare strings.Compare comparator is slices.IsSorted spelled the slow
// way; the rewrites delete this file's ONLY strings references, so the fix
// also drops the orphaned strings import. The slice expression is kept
// verbatim.

func sortedLiteral(xs []string) bool {
	return slices.IsSortedFunc(xs, func(a, b string) int { return strings.Compare(a, b) }) // want `slices\.IsSortedFunc with a bare strings\.Compare comparator`
}

// strings.Compare passed directly (one fewer call layer) is matched too.
func sortedFuncValue(xs []string) bool {
	return slices.IsSortedFunc(xs, strings.Compare) // want `slices\.IsSortedFunc with a bare strings\.Compare comparator`
}

// Split parameter fields func(a string, b string) match like func(a, b string),
// and the slice expression survives verbatim however it is spelled.
func sortedSplitParams(m map[string][]string, key string) bool {
	return slices.IsSortedFunc(m[key], func(a string, b string) int { return strings.Compare(a, b) }) // want `slices\.IsSortedFunc with a bare strings\.Compare comparator`
}

// An explicit instantiation keeps its brackets: slices.IsSorted has the same
// two type parameters, and string is cmp.Ordered.
func sortedInstantiated(xs []string) bool {
	return slices.IsSortedFunc[[]string, string](xs, strings.Compare) // want `slices\.IsSortedFunc with a bare strings\.Compare comparator`
}
