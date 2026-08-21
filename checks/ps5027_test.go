package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5027(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5027.Analyzer, "ps5027")
}
