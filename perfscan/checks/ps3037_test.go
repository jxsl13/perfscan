package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3037(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3037.Analyzer, "ps3037")
}
func TestPS3037Orphan(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3037.Analyzer, "ps3037orphan")
}
func TestPS3037Add(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3037.Analyzer, "ps3037add")
}
