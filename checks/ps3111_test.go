package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3111(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3111.Analyzer, "ps3111")
}
