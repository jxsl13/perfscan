package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3028(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3028.Analyzer, "ps3028")
}
