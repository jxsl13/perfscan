package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5087(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5087.Analyzer, "ps5087", "ps5087alias")
}
