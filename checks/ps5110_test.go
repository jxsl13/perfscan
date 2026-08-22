package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5110(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5110.Analyzer,
		"ps5110", "ps5110alias", "ps5110comment", "ps5110dot")
}
