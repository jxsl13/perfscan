package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5096(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5096.Analyzer,
		"ps5096", "ps5096alias", "ps5096comment", "ps5096dot", "ps5096shadow")
}
