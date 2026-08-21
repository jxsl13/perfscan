package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3016(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3016.Analyzer, "ps3016")
}
