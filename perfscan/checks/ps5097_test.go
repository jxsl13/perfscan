package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5097(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5097.Analyzer,
		"ps5097", "ps5097alias", "ps5097comment", "ps5097dot", "ps5097localconst")
}
