package ps3002

// Only "sort" is imported: the single-field rewrite needs slices AND cmp
// and the fix ADDS both imports at their sorted positions. The sort.Sort
// call keeps a surviving sort reference, so the golden compiles without
// the runner's orphan pruning (analysistest applies only the check's own
// edits).

import (
	"sort"
)

type person struct {
	age  int
	name string
}

func sortFieldNoImports(xs []person, keep sort.Interface) {
	sort.Sort(keep)
	sort.Slice(xs, func(i, j int) bool { return xs[i].age < xs[j].age }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}
