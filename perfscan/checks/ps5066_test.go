package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5066(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5066.Analyzer, "ps5066")
}
