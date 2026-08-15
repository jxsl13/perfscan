package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3021(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3021.Analyzer, "ps3021")
}
