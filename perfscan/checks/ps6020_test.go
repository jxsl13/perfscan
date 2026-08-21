package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/perfscan/config"
)

func TestPS6020(t *testing.T) {
	defer config.SetForTesting(config.Config{
		PureComputeFuncs:         []string{"groupProjection"},
		LayoutOpConstants:        []string{"OpSlice", "OpTranspose"},
		VariadicDispatchWrappers: []string{"exec"},
	})()
	analysistest.Run(t, analysistest.TestData(), PS6020.Analyzer, "ps6020")
}
