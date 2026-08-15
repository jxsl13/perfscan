package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3020(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3020.Analyzer, "ps3020")
}
