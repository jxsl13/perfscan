package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5052 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. The suite covers slices.Sorted(maps.Keys) and
// maps.Values, a named map type, a comment-inside advisory, and negatives that
// stay SILENT: slices.Sorted of a non-maps iterator, slices.SortedFunc (a user
// comparator is excluded), a plain len(m), and cap instead of len.
func TestPS5052(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5052.Analyzer, "ps5052")
}
