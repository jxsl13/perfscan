package ps3002

// The only sort reference in this FILE is the fixable call itself: the
// rewrite would orphan the import (the fix runner never prunes imports),
// so the finding stays advisory here.

import "sort"

type orphanItem struct{ key int }

func sortOrphan(xs []orphanItem) {
	sort.Slice(xs, func(i, j int) bool { return xs[i].key < xs[j].key }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}
