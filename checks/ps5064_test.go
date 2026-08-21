package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5064(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5064.Analyzer, "ps5064")
}
