package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5128(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5128.Analyzer,
		"ps5128", "ps5128alias", "ps5128comment", "ps5128dot")
}
