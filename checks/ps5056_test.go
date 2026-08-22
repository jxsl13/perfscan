package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5056(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5056.Analyzer, "ps5056")
}
