package ps3002

import (
	"cmp"
	"slices"
	"sort"
)

type item struct {
	key   int
	other int
	name  string
}

func sortItems(xs []item) {
	sort.Slice(xs, func(i, j int) bool { return xs[i].key < xs[j].key }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

func sortStable(xs []item) {
	sort.SliceStable(xs, func(i, j int) bool { return xs[i].key < xs[j].key }) // want `sort\.SliceStable swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

func sortStrings(xs []string) {
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// Descending order: the left operand indexes with j, not i. Advisory only.
func sortDescending(xs []item) {
	sort.Slice(xs, func(i, j int) bool { return xs[j].key < xs[i].key }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// Different fields on the two sides. Advisory only.
func sortMixedFields(xs []item) {
	sort.Slice(xs, func(i, j int) bool { return xs[i].key < xs[j].other }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// Operands are calls, not plain selector chains. Advisory only.
func sortByLen(xs []item) {
	sort.Slice(xs, func(i, j int) bool { return len(xs[i].name) < len(xs[j].name) }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// '>' comparison, not '<'. Advisory only.
func sortGreater(xs []item) {
	sort.Slice(xs, func(i, j int) bool { return xs[i].key > xs[j].key }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// A local name shadows the cmp package at the call site. Advisory only.
func sortShadowedCmp(xs []item) {
	cmp := 0
	_ = cmp
	sort.Slice(xs, func(i, j int) bool { return xs[i].key < xs[j].key }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

type holder struct{ xs []item }

// The sorted value is a selector, not a plain identifier. Advisory only.
func sortField(h *holder) {
	sort.Slice(h.xs, func(i, j int) bool { return h.xs[i].key < h.xs[j].key }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// Extra statement in the comparator body. Advisory only.
func sortWithStmt(xs []item) {
	sort.Slice(xs, func(i, j int) bool { a, b := xs[i], xs[j]; return a.key < b.key }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

func sortInts(xs []int) {
	sort.Ints(xs)
}

func alreadyModern(xs []int) {
	slices.SortFunc(xs, func(a, b int) int { return cmp.Compare(a, b) })
}
