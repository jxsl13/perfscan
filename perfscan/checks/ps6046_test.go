package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6046(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6046.Analyzer, "ps6046")
}
