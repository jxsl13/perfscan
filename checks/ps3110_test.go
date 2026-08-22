package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3110(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3110.Analyzer, "ps3110")
}
