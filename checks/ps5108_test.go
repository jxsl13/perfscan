package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5108(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5108.Analyzer,
		"ps5108", "ps5108alias", "ps5108comment", "ps5108dot")
}
