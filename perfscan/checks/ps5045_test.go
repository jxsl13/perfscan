package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5045 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. Positives cover maps.Keys, maps.Values, and a named
// map type. Advisory: a comment in the deleted scaffolding. Negatives stay
// SILENT: slices.Collect of a non-maps iterator, a plain len(m), and cap
// instead of len. See equiv_PS5045_test.go for the runtime proof.
func TestPS5045(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5045.Analyzer, "ps5045")
}
