package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5080(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5080.Analyzer, "ps5080", "ps5080alias")
}
