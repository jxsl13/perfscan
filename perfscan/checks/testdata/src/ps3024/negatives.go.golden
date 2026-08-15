package ps3024

import "sort"

// Every call in this file stays ADVISORY (reported, no fix): the closure or
// the target falls outside the one provably bit-identical shape. The golden
// is identical, and the sort import survives.

type pair struct{ k, v int }

// DESCENDING whole-element closure: that predicate is "is sorted
// descending" — slices.IsSortedFunc territory, not slices.IsSorted.
func descending(xs []int) bool {
	return sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] > xs[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

// Swapped operand order — descending in disguise (left indexes with j).
func swapped(xs []int) bool {
	return sort.SliceIsSorted(xs, func(i, j int) bool { return xs[j] < xs[i] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

// Key extraction: sortedness by a FIELD is slices.IsSortedFunc territory.
func byField(xs []pair) bool {
	return sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i].k < xs[j].k }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

// Non-strict <=: a broken less contract with DIFFERENT results on ties —
// [2, 2] is "sorted" under slices.IsSorted but this closure returns false.
func nonStrict(xs []int) bool {
	return sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] <= xs[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

// FLOAT elements: cmp.Less orders NaN as the smallest value while < treats
// NaN as incomparable — the family's bright no-float line.
func floats(fs []float64) bool {
	return sort.SliceIsSorted(fs, func(i, j int) bool { return fs[i] < fs[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

// The closure indexes a DIFFERENT slice than the one under test.
func otherSlice(xs, ys []int) bool {
	return sort.SliceIsSorted(xs, func(i, j int) bool { return ys[i] < ys[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

func supply() []int { return nil }

// The target is a CALL, not a pure path: the closure's re-evaluations could
// have side effects or yield a different slice each time.
func callTarget() bool {
	return sort.SliceIsSorted(supply(), func(i, j int) bool { return supply()[i] < supply()[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

// Extra statement in the closure body.
func extraStmt(xs []int) bool {
	return sort.SliceIsSorted(xs, func(i, j int) bool { v := xs[i] < xs[j]; return v }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

// Untyped nil boxes into sort's any parameter (and is trivially "sorted"),
// but the generic slices.IsSorted(nil) cannot infer its type parameters.
func nilArg() bool {
	return sort.SliceIsSorted(nil, func(i, j int) bool { return false }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

// A non-slice argument: sort.SliceIsSorted panics at runtime, and the
// generic rewrite would not even compile.
func notSlice(xs [4]int) bool {
	return sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] < xs[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

// A comparator that is not a func literal at the call site.
func namedLess(xs []int, less func(i, j int) bool) bool {
	return sort.SliceIsSorted(xs, less) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

// A local name shadows the slices package at the call site (the file does
// not import slices, and the bare name is taken).
func shadowedSlices(xs []int) bool {
	slices := 0
	_ = slices
	return sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] < xs[j] }) // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
}

// A comment inside the closure the fix would delete: never silently drop a
// comment — advisory.
func commented(xs []int) bool {
	return sort.SliceIsSorted(xs, func(i, j int) bool { // want `sort\.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices\.IsSorted/IsSortedFunc scans the concrete type directly`
		// tie-heavy inputs are the common case here
		return xs[i] < xs[j]
	})
}
