package ps1008

import "slices"

// Everything here must stay SILENT: the equality func is not a bare
// x == y on the two parameters, the element types differ, or the callee is
// not the stdlib slices.EqualFunc at all.

func intEq(x, y int) bool { return x == y }

// A named func value could be anything tomorrow — only a fresh literal
// matches.
func negNamedFunc(a, b []int) bool {
	return slices.EqualFunc(a, b, intEq)
}

// Another operator is not equality.
func negNotEq(a, b []int) bool {
	return slices.EqualFunc(a, b, func(x, y int) bool { return x != y })
}

// A negated inequality is equivalent but is not the bare shape — silent.
func negNegated(a, b []int) bool {
	return slices.EqualFunc(a, b, func(x, y int) bool { return !(x != y) })
}

// Field selectors compare PARTS of the elements, not the elements.
type kv struct{ K, V int }

func negFields(a, b []kv) bool {
	return slices.EqualFunc(a, b, func(x, y kv) bool { return x.K == y.K })
}

// A captured variable is not the two parameters.
func negCaptured(a, b []int, z int) bool {
	return slices.EqualFunc(a, b, func(x, y int) bool { return x == z })
}

// The same parameter twice is not a pairwise comparison.
func negSameParam(a, b []int) bool {
	return slices.EqualFunc(a, b, func(x, y int) bool { return x == x })
}

// An extra statement means the closure does more than compare.
func negExtraStmt(a, b []int) bool {
	return slices.EqualFunc(a, b, func(x, y int) bool { z := x; return z == y })
}

// A blank parameter cannot be an operand.
func negBlank(a, b []int) bool {
	return slices.EqualFunc(a, b, func(_ int, y int) bool { return y == y })
}

// Two UNRELATED element types: slices.Equal cannot express the comparison
// at all — silent, not advisory.
func negMixedElems(a []any, b []int) bool {
	return slices.EqualFunc(a, b, func(x any, y int) bool { return x == y })
}

// A same-named method on a shadowing local is not the stdlib function.
type fakeSlices struct{}

func (fakeSlices) EqualFunc(a, b []int, eq func(int, int) bool) bool { return false }

func negShadowed(a, b []int) bool {
	var slices fakeSlices
	return slices.EqualFunc(a, b, func(x, y int) bool { return x == y })
}
