package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5115(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5115.Analyzer,
		"ps5115", "ps5115alias", "ps5115comment", "ps5115dot")
}
