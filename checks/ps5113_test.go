package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5113(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5113.Analyzer,
		"ps5113", "ps5113alias", "ps5113comment", "ps5113dot")
}
