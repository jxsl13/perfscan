package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5058(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5058.Analyzer, "ps5058")
}
func TestPS5058Orphan(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5058.Analyzer, "ps5058orphan")
}
func TestPS5058Add(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5058.Analyzer, "ps5058add")
}
