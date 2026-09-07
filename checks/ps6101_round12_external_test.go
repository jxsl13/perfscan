package checks_test

import (
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6101Round12External(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), checks.PS6101.Analyzer, "ps6101round12")
}
