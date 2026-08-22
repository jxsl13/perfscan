package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3017(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3017.Analyzer, "ps3017")
}
