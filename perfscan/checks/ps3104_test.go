package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3104(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3104.Analyzer, "ps3104")
}
