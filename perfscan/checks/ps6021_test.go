package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6021(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6021.Analyzer, "ps6021")
}
