package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2018 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite forms (verbatim
// seed and count expressions — variables, fields, compound expressions
// with calls, named constants, parenthesized shapes — and nested
// matches), the import surgery (add strings, keep bytes, drop bytes,
// in-place swap, alias reuse), and the advisory guards (non-constant
// count, negative constant count, shadowed strings, comments in the
// replaced punctuation). See equiv_PS2018_test.go for the runtime proof
// that the rewrite is byte-identical on every input and that both forms
// panic on exactly the same inputs.
func TestPS2018(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2018.Analyzer, "ps2018")
}

// A cgo file's import block must never be edited: the fix would need to
// add strings (and drop the orphaned bytes), so it is withheld and the
// report stays advisory — the golden is identical to the source.
func TestPS2018Cgo(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2018.Analyzer, "ps2018cgo")
}

// Negative shapes — named types, real byte-slice operands, defined
// slice conversions, shadowed packages, no outer string conversion —
// must produce no diagnostics at all.
func TestPS2018Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2018.Analyzer, "ps2018neg")
}
