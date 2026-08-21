package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6041(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6041.Analyzer, "ps6041")
}
