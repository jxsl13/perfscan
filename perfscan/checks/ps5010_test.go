package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5010 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite forms for all four
// functions (verbatim operand expressions, nested matches, parenthesized
// shapes), the import surgery (add strings, keep bytes, drop bytes,
// in-place swap, alias reuse), and the advisory guards (shadowed
// strings, comments in the replaced punctuation). See
// equiv_PS5010_test.go for the runtime proof that the rewrite is
// byte-identical on every input.
func TestPS5010(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5010.Analyzer, "ps5010")
}

// A cgo file's import block must never be edited: the fix would need to
// add strings (and drop the orphaned bytes), so it is withheld and the
// report stays advisory — the golden is identical to the source.
func TestPS5010Cgo(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5010.Analyzer, "ps5010cgo")
}

// Negative shapes — named types, byte-slice operands, the *Special
// variants, shadowed packages — must produce no diagnostics at all.
func TestPS5010Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5010.Analyzer, "ps5010neg")
}
