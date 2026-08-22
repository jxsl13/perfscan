package ps3002

// slices is already imported (and used) but cmp is NOT: the field rewrite
// adds ONLY the cmp import and reuses the existing slices one. The
// sort.Sort call keeps a surviving sort reference, so the golden compiles
// without the runner's orphan pruning.

import (
	"slices"
	"sort"
)

type record struct{ id int }

func minOf(xs []int) int { return slices.Min(xs) }

func sortRecords(xs []record, keep sort.Interface) {
	sort.Sort(keep)
	sort.Slice(xs, func(i, j int) bool { return xs[i].id < xs[j].id }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}
