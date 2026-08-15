package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3025(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3025.Analyzer, "ps3025")
}
