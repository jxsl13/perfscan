package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5075(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5075.Analyzer, "ps5075", "ps5075alias")
}
