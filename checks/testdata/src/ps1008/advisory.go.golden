package ps1008

import "slices"

type IntsA []int

// EXPLICIT INSTANTIATION stays advisory: EqualFunc's four type arguments
// (S1, S2, E1, E2) do not transfer to Equal's two (S, E), and with the
// argument list gone the arguments might not survive inference (untyped
// nil). Both the partial single-bracket and the multi-bracket spelling.
func eqInstantiated1(a, b []int) bool {
	return slices.EqualFunc[[]int](a, b, func(x, y int) bool { return x == y }) // want `slices\.Equal compares the int elements with the identical result and the comparison inlined \(no auto-fix: EqualFunc's explicit type arguments do not transfer to Equal's two type parameters\)`
}

func eqInstantiated2(a, b []int) bool {
	return slices.EqualFunc[[]int, []int](a, b, func(x, y int) bool { return x == y }) // want `slices\.Equal compares the int elements with the identical result and the comparison inlined \(no auto-fix: EqualFunc's explicit type arguments do not transfer to Equal's two type parameters\)`
}

// UNTYPED NIL arguments under an explicit instantiation resolve to no
// slice type at all — the classifier stays SILENT (nothing to hold the
// advice to).
func eqInstantiatedNil() bool {
	return slices.EqualFunc[[]int, []int](nil, nil, func(x, y int) bool { return x == y })
}

// SAME element type but DIFFERENT slice types: slices.Equal wants one S,
// so the mechanical rewrite is not guaranteed — advisory only.
func eqNamedVsUnnamed(a IntsA, b []int) bool {
	return slices.EqualFunc(a, b, func(x, y int) bool { return x == y }) // want `slices\.Equal compares the int elements with the identical result and the comparison inlined \(no auto-fix: the slice arguments have different types\)`
}

// A comment inside the span the fix would delete withholds the fix (it
// would be silently destroyed); the report stays advisory.
func eqComment(a, b []int) bool {
	return slices.EqualFunc(a, b /* deliberate */, func(x, y int) bool { return x == y }) // want `slices\.Equal compares the int elements with the identical result and the comparison inlined`
}
