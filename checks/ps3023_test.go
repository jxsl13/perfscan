package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3023(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3023.Analyzer, "ps3023")
}
