package ps3002

// The sort package is imported under an ALIAS: PkgFuncCall matches aliased
// stdlib imports, and the whole s.Slice call is replaced by a slices.SortFunc
// call that never references the alias, so the orphaned `s "sort"` import is
// pruned exactly like the plain-named one in orphan.go.

import (
	s "sort"
)

type aliasedItem struct{ key int }

func sortAliased(xs []aliasedItem) {
	s.Slice(xs, func(i, j int) bool { return xs[i].key < xs[j].key }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}
