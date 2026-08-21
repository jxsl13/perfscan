package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6044(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6044.Analyzer, "ps6044")
}
