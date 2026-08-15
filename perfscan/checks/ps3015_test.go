package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3015(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3015.Analyzer, "ps3015")
}
