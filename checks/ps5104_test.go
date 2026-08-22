package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5104(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5104.Analyzer, "ps5104")
}
