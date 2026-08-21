package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3013(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3013.Analyzer, "ps3013")
}
