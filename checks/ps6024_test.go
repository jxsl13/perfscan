package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6024(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6024.Analyzer, "ps6024")
}
