package ps3002

// slices is DOT-imported: the rewrite has no usable package name for it
// (a bare Sort/SortFunc call would be too fragile), so the report stays
// advisory and the golden is identical — no rewrite, no import edits.

import (
	. "slices"
	"sort"
)

type dotItem struct{ n int }

func dotUse(xs []int) int { return Min(xs) }

func sortDot(xs []dotItem) {
	sort.Slice(xs, func(i, j int) bool { return xs[i].n < xs[j].n }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}
