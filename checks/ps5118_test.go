package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5118(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5118.Analyzer,
		"ps5118", "ps5118alias", "ps5118comment", "ps5118dot")
}
