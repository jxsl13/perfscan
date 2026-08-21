package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5077(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5077.Analyzer, "ps5077", "ps5077alias")
}
