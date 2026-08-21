package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5062(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5062.Analyzer, "ps5062")
}
