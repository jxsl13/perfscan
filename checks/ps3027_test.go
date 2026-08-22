package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3027(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3027.Analyzer, "ps3027")
}
