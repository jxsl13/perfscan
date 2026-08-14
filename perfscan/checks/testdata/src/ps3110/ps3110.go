package ps3110

import "slices"

type User struct{ ID int }

var limit = 10

// --- POSITIVES: bare x == target, call-free target, fixed ---

func idx(s []int, t int) int {
	return slices.IndexFunc(s, func(x int) bool { return x == t }) // want `slices\.IndexFunc with a bare x == target closure`
}

// ContainsFunc, symmetric spelling (target == x).
func has(s []string, name string) bool {
	return slices.ContainsFunc(s, func(x string) bool { return name == x }) // want `slices\.ContainsFunc with a bare x == target closure`
}

// A field target (call-free) is kept verbatim.
func field(s []int, c struct{ v int }) int {
	return slices.IndexFunc(s, func(x int) bool { return x == c.v }) // want `slices\.IndexFunc with a bare x == target closure`
}

// A literal target.
func lit(s []int) int {
	return slices.IndexFunc(s, func(x int) bool { return x == 42 }) // want `slices\.IndexFunc with a bare x == target closure`
}

// A package-level variable target — arithmetic over it stays call-free.
func pkgVar(s []int) int {
	return slices.IndexFunc(s, func(x int) bool { return x == limit+1 }) // want `slices\.IndexFunc with a bare x == target closure`
}

// --- NEGATIVES: not reported ---

// A FIELD comparison on the element (u.ID == t) is not a whole-element ==;
// slices.Index cannot express it.
func byField(s []User, t int) int {
	return slices.IndexFunc(s, func(u User) bool { return u.ID == t })
}

// A CALL in the target: the closure evaluates it per element, Index once.
func withCall(s []int) int {
	return slices.IndexFunc(s, func(x int) bool { return x == compute() })
}

func compute() int { return 1 }

// Both operands are the parameter — degenerate, no target.
func selfEq(s []int) int {
	return slices.IndexFunc(s, func(x int) bool { return x == x })
}

// A named func value, not a fresh literal.
func isZero(x int) bool { return x == 0 }

func named(s []int) int {
	return slices.IndexFunc(s, isZero)
}

// Already the direct call.
func direct(s []int, t int) int {
	return slices.Index(s, t)
}
