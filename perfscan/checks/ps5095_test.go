package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5095(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5095.Analyzer, "ps5095", "ps5095alias", "ps5095comment", "ps5095dot", "ps5095orphan")
}
