package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3032(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3032.Analyzer, "ps3032")
}
