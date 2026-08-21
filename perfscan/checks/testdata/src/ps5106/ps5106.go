package ps5106

import "strings"

// All six operators with strings.Compare on the LEFT.
func eq(a, b string) bool  { return strings.Compare(a, b) == 0 } // want `strings\.Compare`
func neq(a, b string) bool { return strings.Compare(a, b) != 0 } // want `strings\.Compare`
func lt(a, b string) bool  { return strings.Compare(a, b) < 0 }  // want `strings\.Compare`
func le(a, b string) bool  { return strings.Compare(a, b) <= 0 } // want `strings\.Compare`
func gt(a, b string) bool  { return strings.Compare(a, b) > 0 }  // want `strings\.Compare`
func ge(a, b string) bool  { return strings.Compare(a, b) >= 0 } // want `strings\.Compare`

// strings.Compare on the RIGHT of 0: the operator is mirrored.
func rlt(a, b string) bool { return 0 < strings.Compare(a, b) }  // want `strings\.Compare`
func rge(a, b string) bool { return 0 >= strings.Compare(a, b) } // want `strings\.Compare`
func req(a, b string) bool { return 0 == strings.Compare(a, b) } // want `strings\.Compare`

// Argument shapes: a call and a field selector, spliced verbatim.
type pair struct{ x, y string }

func callArg(p pair, f func() string) bool { return strings.Compare(f(), p.y) == 0 } // want `strings\.Compare`

// A Compare-to-zero call that consumes Clone chains reaches the final direct
// comparison in one pass rather than overlapping with PS5082.
func clonedArgs(a, b string) bool {
	return strings.Compare(strings.Clone(strings.Clone(a)), strings.Clone(b)) == 0 // want `final rewrite also removes 3 throwaway strings\.Clone layer`
}

// NEGATIVE: compared to a non-zero literal — not a Compare-to-zero shape.
func nonZero(a, b string) bool { return strings.Compare(a, b) == 1 }

// NEGATIVE: the result is bound and compared later, out of scope.
func bound(a, b string) int { c := strings.Compare(a, b); return c }

// NEGATIVE: a shadowed strings does not resolve to the stdlib function.
type fakeStrings struct{}

func (fakeStrings) Compare(a, b string) int { return 0 }

func shadowed(a, b string) bool {
	strings := fakeStrings{}
	return strings.Compare(a, b) == 0
}
