package ps3002

// Neither slices nor cmp is imported in this FILE, so the multi-field
// tie-break rewrite (which never adds imports) stays advisory here.

import "sort"

type pair struct {
	first  string
	second string
}

func sortPairsNoImports(xs []pair) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].first != xs[j].first {
			return xs[i].first < xs[j].first
		}
		return xs[i].second < xs[j].second
	})
}
