package checks

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6119(t *testing.T) {
	t.Parallel()
	results := analysistest.Run(t, analysistest.TestData(), ps6119TestAnalyzer([]config.ResidentWeightBenchmarkContract{
		{AcceleratorCallable: "ps6119.Device.Launch", WeightArgument: 2},
		{AcceleratorCallable: "ps6119.Weight.Dispatch", WeightReceiver: true},
	}), "ps6119")
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			if len(diagnostic.SuggestedFixes) != 0 {
				t.Fatal("PS6119 must not suggest a fix")
			}
		}
	}
}

func TestPS6119ContractValidation(t *testing.T) {
	t.Parallel()
	for _, invalid := range []config.ResidentWeightBenchmarkContract{{}, {AcceleratorCallable: "ps6119.Device.Launch"}, {AcceleratorCallable: "Launch", WeightArgument: 2}, {AcceleratorCallable: "ps6119.Device.Launch", WeightArgument: 2, WeightReceiver: true}, {AcceleratorCallable: "ps6119.Weight.Dispatch", WeightArgument: -1, WeightReceiver: true}} {
		if invalid.Valid() {
			t.Fatalf("invalid contract accepted: %+v", invalid)
		}
	}
	duplicates := []config.ResidentWeightBenchmarkContract{{AcceleratorCallable: "ps6119.Device.Launch", WeightArgument: 2}, {AcceleratorCallable: "ps6119.Device.Launch", WeightArgument: 3}}
	if got := ps6119Contracts(duplicates); len(got) != 0 {
		t.Fatalf("ambiguous contracts accepted: %v", got)
	}
}

func TestPS6119EvidenceBoundary(t *testing.T) {
	t.Parallel()
	text := strings.Join(strings.Fields(PS6119.Doc.Text), " ")
	for _, phrase := range []string{"proves neither physical residency nor a cache hit", "working-set bytes", "GPU, encoder, and wall", "whole-workload gate", "equal host/device boundary gate", "There is NO automatic fix"} {
		if !strings.Contains(text, phrase) {
			t.Errorf("missing %q", phrase)
		}
	}
}

func ps6119TestAnalyzer(contracts []config.ResidentWeightBenchmarkContract) *analysis.Analyzer {
	analyzer := *PS6119.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) { return runPS6119WithContracts(pass, contracts) }
	return &analyzer
}
