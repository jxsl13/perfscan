package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5057(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5057.Analyzer, "ps5057")
}
