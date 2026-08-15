package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3011(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3011.Analyzer, "ps3011")
}
