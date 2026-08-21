package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5124(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5124.Analyzer,
		"ps5124", "ps5124alias", "ps5124comment", "ps5124dot")
}
