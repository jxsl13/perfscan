package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3022(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3022.Analyzer, "ps3022")
}
