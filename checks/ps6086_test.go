package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6086(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6086.Analyzer, "ps6086", "ps6086old", "ps6086iter")
}
