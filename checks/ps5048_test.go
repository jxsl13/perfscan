package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5048 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. Positives cover ==, != and expression arguments (which
// unwrap without parentheses). Advisory: a comment in a removed wrapper, and
// (in ps5048orphan) a rewrite that would orphan the strconv import. Negatives
// stay SILENT: an ordering comparison, a single-Itoa comparison, and
// FormatInt. See equiv_PS5048_test.go for the runtime proof.
func TestPS5048(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5048.Analyzer, "ps5048")
}

// TestPS5048Orphan pins the withheld-fix guard when strconv would be orphaned.
func TestPS5048Orphan(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5048.Analyzer, "ps5048orphan")
}
