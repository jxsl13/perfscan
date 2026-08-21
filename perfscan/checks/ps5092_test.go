package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5092(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5092.Analyzer, "ps5092", "ps5092alias")
}
