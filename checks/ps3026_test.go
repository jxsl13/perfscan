package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3026(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3026.Analyzer, "ps3026")
}
