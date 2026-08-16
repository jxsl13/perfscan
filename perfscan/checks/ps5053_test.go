package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5053 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. The suite covers Quote == / != and concatenated
// arguments that unwrap without parentheses, a comment-inside advisory, and
// negatives that stay SILENT: an ordering comparison, a single Quote operand,
// and QuoteToASCII. See equiv_PS5053_test.go for the byte-identity proof and
// ps5053orphan (the fix is withheld rather than orphaning strconv).
func TestPS5053(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5053.Analyzer, "ps5053")
}

// TestPS5053Orphan pins that when the Quote calls are the file's only strconv
// use, the fix is withheld (advisory) so it never orphans strconv.
func TestPS5053Orphan(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5053.Analyzer, "ps5053orphan")
}
