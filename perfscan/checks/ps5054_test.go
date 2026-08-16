package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5054 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. The suite covers == / != (negated) and expression
// arguments, a comment-inside advisory, and negatives that stay SILENT: an
// ordering comparison and a single hex operand. See equiv_PS5054_test.go for
// the byte-identity proof, ps5054orphan (the fix is withheld rather than
// orphaning hex), and ps5054add (the bytes import is inserted).
func TestPS5054(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5054.Analyzer, "ps5054")
}

func TestPS5054Orphan(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5054.Analyzer, "ps5054orphan")
}

func TestPS5054Add(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5054.Analyzer, "ps5054add")
}
