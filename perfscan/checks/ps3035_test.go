package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3035(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3035.Analyzer, "ps3035")
}
