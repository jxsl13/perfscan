package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5086(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5086.Analyzer, "ps5086", "ps5086alias")
}
