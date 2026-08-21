package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6016(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6016.Analyzer, "ps6016")
}
