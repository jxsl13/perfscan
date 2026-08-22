package ps3002

// Neither slices nor cmp is imported in this FILE: the multi-field
// tie-break rewrite now ADDS both imports at their sorted positions.
// The sort.Sort call keeps a surviving sort reference, so the golden
// compiles without the runner's orphan pruning (analysistest applies
// only the check's own edits).

import (
	"sort"
)

type pair struct {
	first  string
	second string
}

func sortPairsNoImports(xs []pair, keep sort.Interface) {
	sort.Sort(keep)
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].first != xs[j].first {
			return xs[i].first < xs[j].first
		}
		return xs[i].second < xs[j].second
	})
}
