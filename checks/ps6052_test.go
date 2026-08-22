package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6052(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6052.Analyzer, "ps6052")
}
