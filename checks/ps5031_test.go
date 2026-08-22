package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5031 is an AutoFix check: the fixes are applied and diffed against a
// .go.golden fixture. The suite covers all six membership shapes with
// the literal on either side of the operator (mirrored operators
// included), a parenthesized literal, the aliased-import qualifier, the
// ! rewrite composed into a larger && condition (unary ! binds tighter,
// so no parentheses are introduced), the canonical if-condition shape, a
// one-byte NAMED-constant needle (plain Contains rewrite — PS5016
// reports that shape advisory-only, so nothing churns), the bit-identical
// empty-needle edge, and side-effecting arguments passing through
// verbatim. The negatives in the same fixture pin the guards: one-byte
// string-literal needles (PS5007's territory — its LastIndexByte rewrite
// would collide, and a Contains spelling would be PS5016's Before-shape),
// position-dependent comparisons (== 0, != 0, > 0, >= 1), the constant
// comparisons >= -1 / < -1, a bound or indexed result, non-literal
// comparands, a named bool context, bytes.LastIndex, LastIndexByte,
// plain Index (the rejected readability-only rewrite), and a shadowed
// strings identifier. See equiv_PS5031_test.go for the runtime proof
// that the membership predicate is bit-identical on every input.
func TestPS5031(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5031.Analyzer, "ps5031")
}
