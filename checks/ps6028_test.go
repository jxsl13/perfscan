package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6028(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6028.Analyzer, "ps6028")
}
