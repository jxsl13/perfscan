package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5021 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite forms ([]byte and
// []uint8 spellings, a parenthesized conversion type, index/field/slice
// and call operands kept byte-verbatim, untyped constants, named string
// operands, named and type-parameter destinations, expression positions)
// and the advisory guards (defined-type and alias conversion spellings,
// comments in the deleted scaffolding). See equiv_PS5021_test.go for the
// runtime proof that dropping the conversion is byte-identical on every
// input, overlapping unsafe-aliased sources included.
func TestPS5021(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5021.Analyzer, "ps5021")
}

// Negative shapes — plain slice sources, slice->slice and nil conversions,
// []rune, named byte element types (where the rewrite would not even
// type-check), a shadowed copy, a method named copy, and the
// already-rewritten form — must produce no diagnostics at all.
func TestPS5021Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5021.Analyzer, "ps5021neg")
}
