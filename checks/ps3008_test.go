package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3008(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3008.Analyzer, "ps3008")
}
