package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5065(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5065.Analyzer, "ps5065")
}
