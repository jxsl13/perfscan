package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6012(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6012.Analyzer, "ps6012", "ps6012thin")
}
