package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2012 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite forms (verbatim
// operand expressions, nested matches, parenthesized shapes), the import
// surgery (add strings, keep bytes, drop bytes, in-place swap, alias
// reuse), and the advisory guards (shadowed strings, comments in the
// replaced punctuation). See equiv_PS2012_test.go for the runtime proof
// that the rewrite is byte-identical on every input.
func TestPS2012(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2012.Analyzer, "ps2012")
}

// A cgo file's import block must never be edited: the fix would need to
// add strings (and drop the orphaned bytes), so it is withheld and the
// report stays advisory — the golden is identical to the source.
func TestPS2012Cgo(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2012.Analyzer, "ps2012cgo")
}

// Negative shapes — named types, byte-slice operands, other trims — must
// produce no diagnostics at all.
func TestPS2012Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2012.Analyzer, "ps2012neg")
}
