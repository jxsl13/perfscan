package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5114(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5114.Analyzer,
		"ps5114", "ps5114alias", "ps5114comment", "ps5114dot")
}
