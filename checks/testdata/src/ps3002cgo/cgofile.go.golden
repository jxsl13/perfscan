package ps3002cgo

// A cgo file's import block must never be edited, so PS3002 offers no fix
// here at all (not even the call rewrite — whether an import edit is
// needed is a per-file decision, and cgo excludes the whole file): the
// report stays advisory and the golden is identical.

import "C"

import "sort"

type cgoItem struct{ v int }

func sortCgo(xs []cgoItem) {
	sort.Slice(xs, func(i, j int) bool { return xs[i].v < xs[j].v }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}
