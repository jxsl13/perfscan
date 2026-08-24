package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6090(t *testing.T) {
	t.Parallel()
	compute := config.Config{PureComputeFuncs: []string{
		"ps6090.QMatMul",
		"ps6090.ComputeOnly",
		"ps6090.engine.Compute",
		"GenericCompute",
		"ps6090.OnlyError",
		"ps6090.ErrorFirst",
		"ps6090.ProductionCompute",
	}}.Compile().PureComputeFuncs
	analyzer := *PS6090.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6090WithCompute(pass, compute)
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6090")
}

func TestPS6090WithoutVocabulary(t *testing.T) {
	t.Parallel()
	analyzer := *PS6090.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6090WithCompute(pass, nil)
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6090noconfig")
}
