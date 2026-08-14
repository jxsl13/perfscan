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

// Single-field DESCENDING '>': fixed with the cmp.Compare operands SWAPPED —
// cmp.Compare(b.key, a.key) < 0 iff a.key > b.key, the comparator's result.
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

// The sorted value is a selector chain (h.xs) — a side-effect-free path,
// evaluated once as the call argument in both spellings — so it is FIXED,
// splicing the original path spelling back in.
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

// Whole-element []int compare becomes slices.Sort — no cmp import needed.
func sortIntsWhole(xs []int) {
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// SliceStable whole-element collapses to slices.Sort (stable == unstable for a
// basic ordered element, whose equal values are indistinguishable).
func sortIntsStableWhole(xs []int) {
	sort.SliceStable(xs, func(i, j int) bool { return xs[i] < xs[j] }) // want `sort\.SliceStable swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// FLOAT stays advisory: cmp.Compare/slices.Sort order NaN as smallest, but the
// '<' comparator treats NaN as incomparable, so the rewrite is not bit-identical.
func sortFloats(fs []float64) {
	sort.Slice(fs, func(i, j int) bool { return fs[i] < fs[j] }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

type meta struct{ name string }

type gvk struct {
	group   string
	version string
	kind    string
	score   float64
	meta    meta
}

// Two-field ascending tie-break: each guard tests the very field it orders.
func sortTwoFields(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].group != xs[j].group {
			return xs[i].group < xs[j].group
		}
		return xs[i].version < xs[j].version
	})
}

// Three-field ascending tie-break.
func sortThreeFields(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].group != xs[j].group {
			return xs[i].group < xs[j].group
		}
		if xs[i].version != xs[j].version {
			return xs[i].version < xs[j].version
		}
		return xs[i].kind < xs[j].kind
	})
}

// A nested selector chain in the guard field.
func sortNestedChain(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].meta.name != xs[j].meta.name {
			return xs[i].meta.name < xs[j].meta.name
		}
		return xs[i].kind < xs[j].kind
	})
}

// SliceStable multi-field becomes slices.SortStableFunc.
func sortStableTwoFields(xs []gvk) {
	sort.SliceStable(xs, func(i, j int) bool { // want `sort\.SliceStable swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].group != xs[j].group {
			return xs[i].group < xs[j].group
		}
		return xs[i].kind < xs[j].kind
	})
}

// A FLOAT field anywhere in the chain keeps the whole chain advisory.
func sortFloatInChain(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].score != xs[j].score {
			return xs[i].score < xs[j].score
		}
		return xs[i].kind < xs[j].kind
	})
}

// The guard tests one field but orders another: not the canonical tie-break,
// the bool and cmp chains diverge on ties. Advisory only.
func sortGuardMismatch(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].group != xs[j].group {
			return xs[i].version < xs[j].version
		}
		return xs[i].kind < xs[j].kind
	})
}

// A descending '>' in the final field behind an ascending guard: fixed, the
// descending field alone gets swapped operands.
func sortDescendingField(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].group != xs[j].group {
			return xs[i].group < xs[j].group
		}
		return xs[i].kind > xs[j].kind
	})
}

// A guard with an else branch. Advisory only.
func sortGuardElse(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].group != xs[j].group {
			return xs[i].group < xs[j].group
		} else {
			return xs[i].kind < xs[j].kind
		}
	})
}

// A guard body with an extra statement. Advisory only.
func sortGuardExtraStmt(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].group != xs[j].group {
			g := xs[i].group
			return g < xs[j].group
		}
		return xs[i].kind < xs[j].kind
	})
}

// An init statement in the guard. Advisory only.
func sortGuardInit(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if g := xs[i].group; g != xs[j].group {
			return g < xs[j].group
		}
		return xs[i].kind < xs[j].kind
	})
}

// The final compare indexes i/j reversed (descending). Advisory only.
func sortReversedIndex(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].group != xs[j].group {
			return xs[i].group < xs[j].group
		}
		return xs[j].kind < xs[i].kind
	})
}

// The final compare reads a DIFFERENT slice. Advisory only.
func sortDifferentSlice(xs, ys []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].group != xs[j].group {
			return xs[i].group < xs[j].group
		}
		return ys[i].kind < ys[j].kind
	})
}

// MIXED directions: group DESCENDING, then version ASCENDING — each field
// keeps its own operand order in the cmp.Compare chain.
func sortMixedDirections(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].group != xs[j].group {
			return xs[i].group > xs[j].group
		}
		return xs[i].version < xs[j].version
	})
}

// SliceStable single-field descending becomes slices.SortStableFunc with the
// operands swapped.
func sortStableDescending(xs []item) {
	sort.SliceStable(xs, func(i, j int) bool { return xs[i].key > xs[j].key }) // want `sort\.SliceStable swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// Whole-element DESCENDING stays advisory: slices.Sort is ascending only and
// there is no comparator whose operands could be swapped.
func sortIntsWholeDescending(xs []int) {
	sort.Slice(xs, func(i, j int) bool { return xs[i] > xs[j] }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// A descending FLOAT field stays advisory: the NaN exclusion applies to
// every field regardless of direction.
func sortFloatFieldDescending(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { return xs[i].score > xs[j].score }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// The guard tests one field but the '>' return orders ANOTHER: not the
// canonical tie-break, the chains diverge on ties. Advisory only.
func sortGuardMismatchDescending(xs []gvk) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].group != xs[j].group {
			return xs[i].version > xs[j].version
		}
		return xs[i].kind < xs[j].kind
	})
}

// A whole-element compare inside a GUARD body (empty field chain) is declined:
// the tie-break machinery expects a guard to test the very field it then
// orders, so an `xs[i] != xs[j]` guard whose body orders the whole element
// (cmpSuffix == "") hits the distinct empty-chain clause of the line-363 guard
// and stays advisory — the check never emits a fix for this degenerate shape.
// (Sibling of sortGuardMismatchDescending, which exercises the cmpSuffix !=
// condSuffix clause of the same guard.)
func sortWholeElemGuard(xs []int) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i] != xs[j] {
			return xs[i] < xs[j]
		}
		return false
	})
}
