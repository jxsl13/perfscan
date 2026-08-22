package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3036(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3036.Analyzer, "ps3036")
}
