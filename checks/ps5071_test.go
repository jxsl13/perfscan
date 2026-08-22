package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5071 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. Positives cover ==, !=, the Yoda operand order, an
// expression argument (unwraps without parentheses), a canonical negative
// constant, and a named string constant matched by value. Advisory: a comment
// inside the removed wrapper. Negatives stay SILENT: an ordering comparison, a
// non-canonical constant (leading zero, plus sign, non-numeric, empty), an
// int32-overflowing constant, and the Itoa == Itoa shape (PS5048's). The
// orphan guard lives in ps5071orphan. See equiv_PS5071_test.go for the runtime
// proof.
func TestPS5071(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5071.Analyzer, "ps5071")
}

// TestPS5071Orphan pins the withheld-fix guard when strconv would be orphaned.
func TestPS5071Orphan(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5071.Analyzer, "ps5071orphan")
}
