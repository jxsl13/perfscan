package ps3002

// DESCENDING whole-element comparators: `xs[i] > xs[j]` as the comparator's
// sole statement rewrites to slices.SortFunc(xs, func(a, b T) int { return
// cmp.Compare(b, a) }) — cmp.Compare(b, a) < 0 iff b < a iff a > b, the
// identical descending predicate, so under the shared pdqsort the permutation
// is bit-identical (the reasoning PS3105's sort.Reverse form uses). Only
// "sort" is imported in this FILE: the fix ADDS slices AND cmp at their
// sorted positions. The advisory-only negatives below keep surviving sort
// references, so the golden compiles without the runner's orphan pruning
// (analysistest applies only the check's own edits).

import (
	"sort"
)

// []int descending whole element. FIXED: SortFunc + cmp.Compare(b, a).
func descWholeInts(xs []int) {
	sort.Slice(xs, func(i, j int) bool { return xs[i] > xs[j] }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// []string descending whole element. FIXED.
func descWholeStrings(xs []string) {
	sort.Slice(xs, func(i, j int) bool { return xs[i] > xs[j] }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// SliceStable descending whole element collapses to the UNSTABLE SortFunc:
// equal basic elements are bitwise-identical, so the descending arrangement
// of the multiset is unique and stability is unobservable — the same
// collapse ascending SliceStable makes to slices.Sort. FIXED.
func descWholeStable(xs []int) {
	sort.SliceStable(xs, func(i, j int) bool { return xs[i] > xs[j] }) // want `sort\.SliceStable swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

type bucket struct{ items []int }

// Descending whole element over a SELECTOR target c.items — the recent
// pathExpr/ps3002SamePath support applies unchanged. FIXED, splicing the
// original path spelling back in.
func descWholeSelector(c *bucket) {
	sort.Slice(c.items, func(i, j int) bool { return c.items[i] > c.items[j] }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// A NAMED basic element type declared in this package is locally spellable,
// so the emitted func literal spells it verbatim. FIXED.
type id int

func descWholeNamed(xs []id) {
	sort.Slice(xs, func(i, j int) bool { return xs[i] > xs[j] }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// ASCENDING whole element still becomes slices.Sort — no comparator, no cmp.
func ascWholeStillSort(xs []int) {
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// NEGATIVE: FLOAT stays advisory regardless of direction — cmp.Compare
// orders NaN as the smallest value while '>' treats it as incomparable.
func descWholeFloats(fs []float64) {
	sort.Slice(fs, func(i, j int) bool { return fs[i] > fs[j] }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// NEGATIVE: `>=` is not a strict order (an invalid sort.Slice comparator to
// begin with) — only '<' and '>' are accepted. Advisory only.
func descWholeGeq(xs []int) {
	sort.Slice(xs, func(i, j int) bool { return xs[i] >= xs[j] }) // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
}

// NEGATIVE: the whole-element compare sits in a NON-final statement (an if
// condition), not as the sole `return xs[i] > xs[j]`. A whole-element
// compare is only accepted as the comparator's sole final return — guards
// must name the field they order. Advisory only.
func descWholeNonFinal(xs []int) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i] > xs[j] {
			return true
		}
		return false
	})
}
