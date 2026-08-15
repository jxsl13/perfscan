package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3018(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3018.Analyzer, "ps3018")
}
