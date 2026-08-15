package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2016 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite forms for all three
// functions (verbatim operand and predicate expressions — named funcs,
// method values, func literals with side effects, closures from calls —
// nested matches, parenthesized shapes), the import surgery (add
// strings, keep bytes, drop bytes, in-place swap, alias reuse), and the
// advisory guards (shadowed strings, comments in the replaced
// punctuation). See equiv_PS2016_test.go for the runtime proof that the
// rewrite is byte-identical on every input and preserves the predicate's
// call sequence exactly.
func TestPS2016(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2016.Analyzer, "ps2016")
}

// A cgo file's import block must never be edited: the fix would need to
// add strings (and drop the orphaned bytes), so it is withheld and the
// report stays advisory — the golden is identical to the source.
func TestPS2016Cgo(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2016.Analyzer, "ps2016cgo")
}

// Negative shapes — named types, byte-slice operands, the cutset Trim
// family (PS5005's shape), IndexFunc/LastIndexFunc, shadowed packages —
// must produce no diagnostics at all.
func TestPS2016Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2016.Analyzer, "ps2016neg")
}
