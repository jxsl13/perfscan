package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5073(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5073.Analyzer, "ps5073", "ps5073alias")
}
