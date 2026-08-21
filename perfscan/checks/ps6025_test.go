package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6025(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6025.Analyzer, "ps6025")
}
