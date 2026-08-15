package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3014(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3014.Analyzer, "ps3014")
}
