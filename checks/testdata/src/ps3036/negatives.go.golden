package ps3036

import (
	"slices"
	"strings"
)

type StringSlice []string

// MIXED SLICE TYPES: advisory only (reported, NOT fixed) — CompareFunc takes
// two independently-typed slices, slices.Compare one type S for both, so the
// rewrite would not typecheck.
func mixed(a []string, b StringSlice) int {
	return slices.CompareFunc(a, b, strings.Compare) // want `slices\.CompareFunc with a bare strings\.Compare comparator .* \(no auto-fix: the slice types`
}

// EXPLICIT INSTANTIATION: advisory only — CompareFunc has four type
// parameters, Compare two, so the brackets cannot survive the rewrite.
func instantiated(a, b []string) int {
	return slices.CompareFunc[[]string, []string, string, string](a, b, strings.Compare) // want `slices\.CompareFunc with a bare strings\.Compare comparator .* \(no auto-fix: explicit instantiation`
}

// A comment inside the deleted span would be destroyed: advisory only.
func commented(a, b []string) int {
	return slices.CompareFunc(a, b, strings.Compare /* three-way */) // want `slices\.CompareFunc with a bare strings\.Compare comparator`
}

// Swapped operands = slices.Compare(b, a), a DIFFERENT result — never matched.
func swapped(a, b []string) int {
	return slices.CompareFunc(a, b, func(x, y string) int { return strings.Compare(y, x) })
}

// A defined string type needs conversions to feed strings.Compare, and a
// conversion around a parameter is not the bare parameter — never matched.
type name string

func converted(a, b []name) int {
	return slices.CompareFunc(a, b, func(x, y name) int { return strings.Compare(string(x), string(y)) })
}

// A field selector, not the bare parameters.
type rec struct{ key string }

func byField(a, b []rec) int {
	return slices.CompareFunc(a, b, func(x, y rec) int { return strings.Compare(x.key, y.key) })
}

// A named func value, not a fresh literal or strings.Compare itself.
func myCmp(x, y string) int { return strings.Compare(x, y) }

func named(a, b []string) int {
	return slices.CompareFunc(a, b, myCmp)
}

// Extra computation in the body fails the match (a side effect would be lost).
func extraWork(a, b []string) int {
	calls := 0
	d := slices.CompareFunc(a, b, func(x, y string) int { calls++; return strings.Compare(x, y) })
	_ = calls
	return d
}

// A captured outer variable instead of the second parameter.
func captured(a, b []string, pivot string) int {
	return slices.CompareFunc(a, b, func(x, _y string) int { return strings.Compare(x, pivot) })
}

// Already the direct call — nothing to do.
func direct2(a, b []string) int {
	return slices.Compare(a, b)
}

// A shadowed strings is not the stdlib package — never matched.
type fakeStrings struct{}

func (fakeStrings) Compare(x, y string) int { return 0 }

func shadowed(a, b []string) int {
	var strings fakeStrings
	return slices.CompareFunc(a, b, strings.Compare)
}
