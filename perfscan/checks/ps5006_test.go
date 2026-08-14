package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5006 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite forms for both
// functions (verbatim operand and prefix expressions, nested matches,
// parenthesized shapes), the import surgery (add strings, keep bytes,
// drop bytes, in-place swap, alias reuse), and the advisory guards
// (shadowed strings, comments in the replaced punctuation). See
// equiv_PS5006_test.go for the runtime proof that the rewrite is
// byte-identical on every input.
func TestPS5006(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5006.Analyzer, "ps5006")
}

// A cgo file's import block must never be edited: the fix would need to
// add strings (and drop the orphaned bytes), so it is withheld and the
// report stays advisory — the golden is identical to the source.
func TestPS5006Cgo(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5006.Analyzer, "ps5006cgo")
}

// Negative shapes — named types, genuine []byte operands on either
// side, the cutset Trim family (PS5005's shape), shadowed packages —
// must produce no diagnostics at all.
func TestPS5006Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5006.Analyzer, "ps5006neg")
}
