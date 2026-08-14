package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3109(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3109.Analyzer, "ps3109")
}
