package runner

import (
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

func TestPS6131Vocabulary(t *testing.T) {
	t.Parallel()
	check := &lint.Check{ID: "PS6131", NeedsConfig: true, Vocab: []string{"dispatchCrossoverContracts"}}
	c := config.DispatchCrossoverContract{DispatchFunction: "example/cpu.dispatch", ThresholdConstant: "example/cpu.boundary", SerialPolicy: "example/cpu.leaf", ParallelPolicy: "example/cpu.parallel", WorkerRunner: "example/cpu.parallel", LeafKernels: []string{"example/cpu.nativeLeaf"}, OperationInputFactory: "example/cpu.inputs", DiagnosticFunction: "example/cpu.TestDiagnostic", ProductionBenchmark: "BenchmarkAbs/n{n}", Campaign: "/retained/campaign", PlanSHA256: strings.Repeat("a", 64), RecordsSHA256: strings.Repeat("b", 64)}
	unpinned := c
	unpinned.PlanSHA256 = ""
	for _, tc := range []struct {
		name string
		cfg  config.Config
		want int
	}{
		{"empty", config.Config{}, 1},
		{"complete_pinned", config.Config{DispatchCrossoverContracts: []config.DispatchCrossoverContract{c}}, 0},
		{"unpinned", config.Config{DispatchCrossoverContracts: []config.DispatchCrossoverContract{unpinned}}, 1},
		{"ambiguous_duplicate", config.Config{DispatchCrossoverContracts: []config.DispatchCrossoverContract{c, c}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := missingVocab(check, &tc.cfg); len(got) != tc.want {
				t.Fatalf("missing=%v want count%d", got, tc.want)
			}
		})
	}
}
