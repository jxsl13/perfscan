package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3019(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3019.Analyzer, "ps3019")
}
