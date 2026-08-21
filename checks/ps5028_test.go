package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5028(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5028.Analyzer, "ps5028")
}
