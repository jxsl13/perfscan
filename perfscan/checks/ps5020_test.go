package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5020 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite for the canonical
// shape, untyped string literals, the []uint8 spelling and a []byte
// alias, named string operands, named byte-slice destinations, operand
// expressions including calls (evaluated exactly once in both forms, so
// the fix applies), parenthesized variants, and the advisory guard (a
// comment inside the deleted conversion scaffolding). See
// equiv_PS5020_test.go for the runtime proof that the rewrite is
// bit-identical on every input.
func TestPS5020(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5020.Analyzer, "ps5020")
}

// Negative shapes — direct spreads, []byte-to-[]byte no-op conversions,
// named byte-slice conversion types, composite literals, function calls
// that look like conversions, []rune conversions, []byte(nil), generic
// operands and a shadowed append — must produce no diagnostics at all.
func TestPS5020Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5020.Analyzer, "ps5020neg")
}
