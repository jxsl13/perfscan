package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3033(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3033.Analyzer, "ps3033")
}
