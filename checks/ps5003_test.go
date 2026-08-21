package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5003(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5003.Analyzer, "ps5003")
}
