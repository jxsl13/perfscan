package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6103(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6103.Analyzer, "ps6103", "ps6103local")
}
