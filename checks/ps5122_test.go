package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5122(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5122.Analyzer,
		"ps5122", "ps5122alias", "ps5122comment", "ps5122dot")
}
