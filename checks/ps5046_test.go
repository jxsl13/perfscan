package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5046 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. Positives cover both operand orders, == (negated) and
// != arms, and the Find/FindIndex/FindSubmatch/FindString*/nil-left forms.
// Advisory: a comment in the nil comparison. Negatives stay SILENT: FindString
// (returns a string, compared to ""), a used result, and the two-argument
// FindAll*. See equiv_PS5046_test.go for the runtime proof.
func TestPS5046(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5046.Analyzer, "ps5046")
}
