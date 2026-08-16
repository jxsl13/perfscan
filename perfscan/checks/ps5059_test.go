package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5059(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5059.Analyzer, "ps5059")
}

func TestPS5059Add(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5059.Analyzer, "ps5059add")
}
