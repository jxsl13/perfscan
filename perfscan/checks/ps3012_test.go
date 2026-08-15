package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3012(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3012.Analyzer, "ps3012")
}
