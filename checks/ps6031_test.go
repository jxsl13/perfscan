package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6031(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6031.Analyzer, "ps6031")
}
