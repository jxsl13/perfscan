package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5107(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5107.Analyzer,
		"ps5107", "ps5107alias", "ps5107comment", "ps5107dot")
}
