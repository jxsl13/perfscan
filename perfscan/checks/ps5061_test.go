package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5061(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5061.Analyzer, "ps5061")
}
