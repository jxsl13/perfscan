package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5063(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5063.Analyzer, "ps5063")
}
