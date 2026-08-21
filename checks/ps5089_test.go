package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5089(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5089.Analyzer, "ps5089", "ps5089alias")
}
