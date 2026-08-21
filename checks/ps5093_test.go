package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5093(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5093.Analyzer, "ps5093", "ps5093alias", "ps5093shadow")
}
