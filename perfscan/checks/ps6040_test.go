package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6040(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6040.Analyzer, "ps6040")
}
