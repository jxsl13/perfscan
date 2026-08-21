package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6023(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6023.Analyzer, "ps6023")
}
