//go:build cgo

package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6115RealCgoCallable(t *testing.T) {
	t.Parallel()
	contract := ps6115Contract("realCgo", "C.tiny_accel")
	contract.Calls[0].ConfiguredSite = "ps6115cgo.realCgo"
	contract.Calls[0].HostAlternativeCallable = "ps6115cgo.hostForward"
	contract.Calls[0].OperationConstant = "ps6115cgo.opLoss"
	analysistest.Run(t, analysistest.TestData(), ps6115TestAnalyzer([]config.TinySynchronousAcceleratorScreenContract{contract}), "ps6115cgo")
}
