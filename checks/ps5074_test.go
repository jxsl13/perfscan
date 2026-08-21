package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5074(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5074.Analyzer, "ps5074", "ps5074alias")
	analysistest.Run(t, analysistest.TestData(), PS5074.Analyzer, "ps5074terminal")
}
