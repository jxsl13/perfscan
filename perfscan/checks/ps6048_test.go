package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6048(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6048.Analyzer, "ps6048")
}
