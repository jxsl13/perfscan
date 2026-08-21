package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5111(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5111.Analyzer,
		"ps5111", "ps5111alias", "ps5111comment", "ps5111dot")
}
