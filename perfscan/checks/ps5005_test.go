package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5005 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite forms for all three
// functions (verbatim operand and cutset expressions, nested matches,
// parenthesized shapes), the import surgery (add strings, keep bytes,
// drop bytes, in-place swap, alias reuse), and the advisory guards
// (shadowed strings, comments in the replaced punctuation). See
// equiv_PS5005_test.go for the runtime proof that the rewrite is
// byte-identical on every input.
func TestPS5005(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5005.Analyzer, "ps5005")
}

// A cgo file's import block must never be edited: the fix would need to
// add strings (and drop the orphaned bytes), so it is withheld and the
// report stays advisory — the golden is identical to the source.
func TestPS5005Cgo(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5005.Analyzer, "ps5005cgo")
}

// Negative shapes — named types, byte-slice operands, TrimPrefix/
// TrimSuffix, shadowed packages — must produce no diagnostics at all.
func TestPS5005Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5005.Analyzer, "ps5005neg")
}
