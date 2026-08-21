package ps3011

import (
	"slices"
	"strings"
)

type rec struct{ key string }

// Swapped operands = descending order — never matched (a different search).
func descending(xs []string, t string) (int, bool) {
	return slices.BinarySearchFunc(xs, t, func(a, b string) int { return strings.Compare(b, a) })
}

// A field selector, not the bare parameters.
func byField(xs []rec, t rec) (int, bool) {
	return slices.BinarySearchFunc(xs, t, func(a, b rec) int { return strings.Compare(a.key, b.key) })
}

// A conversion around a parameter is not the bare parameter.
type name string

func converted(xs []name, t name) (int, bool) {
	return slices.BinarySearchFunc(xs, t, func(a, b name) int { return strings.Compare(string(a), string(b)) })
}

// A named func value, not a fresh literal or strings.Compare itself.
func myCmp(a, b string) int { return strings.Compare(a, b) }

func named(xs []string, t string) (int, bool) {
	return slices.BinarySearchFunc(xs, t, myCmp)
}

// Extra computation in the body fails the match (a side effect would be lost).
func extraWork(xs []string, t string) (n int, ok bool) {
	calls := 0
	n, ok = slices.BinarySearchFunc(xs, t, func(a, b string) int { calls++; return strings.Compare(a, b) })
	_ = calls
	return n, ok
}

// A captured outer variable instead of the second parameter.
func captured(xs []string, t string) (int, bool) {
	return slices.BinarySearchFunc(xs, t, func(a, _b string) int { return strings.Compare(a, t) })
}

// Already the direct call — nothing to do.
func direct(xs []string, t string) (int, bool) {
	return slices.BinarySearch(xs, t)
}

// An explicit instantiation: BinarySearchFunc's THREE type arguments do not
// transfer to BinarySearch's two — reported, but advisory (not auto-fixed).
func instantiated(xs []string, t string) (int, bool) {
	return slices.BinarySearchFunc[[]string, string, string](xs, t, strings.Compare) // want `slices\.BinarySearchFunc with a bare strings\.Compare comparator .* \(no auto-fix: an explicit instantiation`
}
