package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6029(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6029.Analyzer, "ps6029")
}
