package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5088(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5088.Analyzer, "ps5088", "ps5088alias")
}
