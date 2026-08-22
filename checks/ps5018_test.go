package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5018 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite for both arms
// (verbatim haystack expressions, a named string haystack, a
// parenthesized mapping argument, nested matches), the import surgery
// (drop the orphaned unicode import, keep it while other references
// remain, reuse aliased strings/unicode spellings), and the advisory
// guard (a comment inside the deleted span). See equiv_PS5018_test.go
// for the runtime proof that the rewrite is value-identical on every
// input.
func TestPS5018(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5018.Analyzer, "ps5018")
}

// A cgo file's import block must never be edited: the fix would need to
// drop the orphaned unicode import, so it is withheld and the report
// stays advisory — the golden is identical to the source.
func TestPS5018Cgo(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5018.Analyzer, "ps5018cgo")
}

// Negative shapes — unicode.ToTitle, SpecialCase method values, wrapper
// funcs, func variables, shadowed packages — must produce no
// diagnostics at all.
func TestPS5018Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5018.Analyzer, "ps5018neg")
}
