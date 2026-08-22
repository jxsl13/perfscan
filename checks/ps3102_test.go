package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3102(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3102.Analyzer, "ps3102")
}
