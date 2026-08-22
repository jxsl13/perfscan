package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6049(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6049.Analyzer, "ps6049")
}
