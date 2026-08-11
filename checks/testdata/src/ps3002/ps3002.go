package ps3002

import "sort"

type item struct{ key int }

func sortItems(xs []item) {
	sort.Slice(xs, func(i, j int) bool { return xs[i].key < xs[j].key }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

func sortStable(xs []item) {
	sort.SliceStable(xs, func(i, j int) bool { return xs[i].key < xs[j].key }) // want `sort\.SliceStable swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

func sortInts(xs []int) {
	sort.Ints(xs)
}
