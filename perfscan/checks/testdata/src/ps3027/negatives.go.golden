package ps3027

import (
	"cmp"
	"slices"
	"strings"
)

// Everything here must stay SILENT: no diagnostic at all.

var order int

// A subtraction comparator can overflow — never matched.
func negSubtraction(a, b []int) int {
	return slices.CompareFunc(a, b, func(x, y int) int { return x - y })
}

// '<=' / '>=' are not the three-way.
func negLessEqual(a, b []int) int {
	return slices.CompareFunc(a, b, func(x, y int) int { if x <= y { return -1 }; if x > y { return 1 }; return 0 })
}

// A swapped sign pair is the REVERSED comparison, not slices.Compare.
func negDescending(a, b []int) int {
	return slices.CompareFunc(a, b, func(x, y int) int { if x < y { return 1 }; if x > y { return -1 }; return 0 })
}

// The same condition twice is not the three-way (no "greater" arm).
func negRepeated(a, b []int) int {
	return slices.CompareFunc(a, b, func(x, y int) int { if x < y { return -1 }; if x < y { return 1 }; return 0 })
}

// An extra statement (a side effect) in the body fails the match.
func negSideEffect(a, b []int) int {
	return slices.CompareFunc(a, b, func(x, y int) int { order++; if x < y { return -1 }; if x > y { return 1 }; return 0 })
}

// A captured variable in a condition fails the match.
func negCapture(a, b []int, pivot int) int {
	return slices.CompareFunc(a, b, func(x, y int) int { if x < pivot { return -1 }; if x > y { return 1 }; return 0 })
}

// A named constant as a result is never accepted (only integer literals).
func negNamedConst(a, b []int) int {
	const less = -1
	return slices.CompareFunc(a, b, func(x, y int) int { if x < y { return less }; if x > y { return 1 }; return 0 })
}

// A default arm other than literal 0 fails the match.
func negNonZeroDefault(a, b []int) int {
	return slices.CompareFunc(a, b, func(x, y int) int { if x < y { return -1 }; if x > y { return 1 }; return 1 })
}

// A NAMED comparator value is not a fresh literal — the func value may be
// shared, hoisted or mutated elsewhere; never matched.
func negNamedComparator(a, b []int) int {
	three := func(x, y int) int { if x < y { return -1 }; if x > y { return 1 }; return 0 }
	return slices.CompareFunc(a, b, three)
}

// A method-call comparator (strings.Compare here) is not the ladder; the
// bare cmp.Compare spelling is PS3023's territory, not PS3027's.
func negOtherComparators(a, b []string, c, d []int) int {
	return slices.CompareFunc(a, b, strings.Compare) + slices.CompareFunc(c, d, cmp.Compare)
}

// A shadowed slices is not the stdlib package.
func negShadowed(a, b []int) int {
	slices := struct {
		CompareFunc func([]int, []int, func(int, int) int) int
	}{CompareFunc: func([]int, []int, func(int, int) int) int { return 0 }}
	return slices.CompareFunc(a, b, func(x, y int) int { if x < y { return -1 }; if x > y { return 1 }; return 0 })
}

// slices.EqualFunc is a different function entirely.
func negEqualFunc(a, b []int) bool {
	return slices.EqualFunc(a, b, func(x, y int) bool { return x == y })
}

// A blank parameter cannot appear in the ladder — no match.
func negBlankParam(a, b []int) int {
	return slices.CompareFunc(a, b, func(x, _ int) int { if x < 0 { return -1 }; if x > 0 { return 1 }; return 0 })
}
