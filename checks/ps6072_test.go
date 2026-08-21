package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6072(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6072.Analyzer, "ps6072")
}
