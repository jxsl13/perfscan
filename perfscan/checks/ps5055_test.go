package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5055(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5055.Analyzer, "ps5055")
}
func TestPS5055Orphan(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5055.Analyzer, "ps5055orphan")
}
func TestPS5055Add(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5055.Analyzer, "ps5055add")
}
