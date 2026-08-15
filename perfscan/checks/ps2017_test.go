package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2017 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite forms (verbatim f
// and s expressions — named func values, multi-line closures, compound
// operands — nested matches, parenthesized shapes, untyped constants),
// the import surgery (add strings, keep bytes, drop bytes, in-place
// swap, alias reuse), and the advisory guards (shadowed strings,
// comments in the replaced punctuation, alias spellings of []byte). See
// equiv_PS2017_test.go for the runtime proof that the rewrite is
// byte-identical on every input and every mapping function.
func TestPS2017(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2017.Analyzer, "ps2017")
}

// A cgo file's import block must never be edited: the fix would need to
// add strings (and drop the orphaned bytes), so it is withheld and the
// report stays advisory — the golden is identical to the source.
func TestPS2017Cgo(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2017.Analyzer, "ps2017cgo")
}

// Negative shapes — named types, byte-slice operands, missing outer
// conversions, shadowed packages — must produce no diagnostics at all.
func TestPS2017Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2017.Analyzer, "ps2017neg")
}
