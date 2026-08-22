package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3105(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3105.Analyzer, "ps3105")
}
