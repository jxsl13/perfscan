package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5119(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5119.Analyzer,
		"ps5119", "ps5119alias", "ps5119comment", "ps5119dot")
}
