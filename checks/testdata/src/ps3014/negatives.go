package ps3014

import (
	"slices"
	"strings"
)

type rec struct{ key string }

// Swapped operands ask whether the slice is DESCENDING — never matched.
func descending(xs []string) bool {
	return slices.IsSortedFunc(xs, func(a, b string) int { return strings.Compare(b, a) })
}

// A field selector, not the bare parameters.
func byField(xs []rec) bool {
	return slices.IsSortedFunc(xs, func(a, b rec) int { return strings.Compare(a.key, b.key) })
}

// A conversion around a parameter is not the bare parameter.
type name string

func converted(xs []name) bool {
	return slices.IsSortedFunc(xs, func(a, b name) int { return strings.Compare(string(a), string(b)) })
}

// A named func value, not a fresh literal or strings.Compare itself.
func myCmp(a, b string) int { return strings.Compare(a, b) }

func named(xs []string) bool {
	return slices.IsSortedFunc(xs, myCmp)
}

// Extra computation in the body fails the match (a side effect would be lost).
func extraWork(xs []string) bool {
	calls := 0
	ok := slices.IsSortedFunc(xs, func(a, b string) int { calls++; return strings.Compare(a, b) })
	_ = calls
	return ok
}

// A captured outer variable instead of the second parameter.
func captured(xs []string, t string) bool {
	return slices.IsSortedFunc(xs, func(a, _b string) int { return strings.Compare(a, t) })
}

// Already the direct call — nothing to do.
func direct(xs []string) bool {
	return slices.IsSorted(xs)
}
