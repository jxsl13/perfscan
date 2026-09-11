package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/config"
)

func TestPS6128MissingVocabularyPermanent(t *testing.T) {
	t.Parallel()
	valid := config.NativeGenerationDispatchContract{Name: "reviewed", CapabilityPredicate: "example.com/p.capable", DispatchVariable: "example.com/p.dispatch", InstalledFunction: "example.com/p.compute", WorkerRunner: "example.com/p.parallel", NativeMainLoopParameter: "k", NativeSymbol: "example.com/p.tile", AssemblyFile: "kernel.s", DescriptorLiteral: "0x5000000000000000", RequiredGenerations: []string{"Apple M2"}, WorkerExecutesSynchronously: true, DescriptorMeaningReviewed: true}
	for _, tc := range []struct {
		name string
		cfg  config.Config
		want int
	}{
		{"empty_contract_warns", config.Config{}, 1},
		{"valid_contract_feeds_check", config.Config{NativeGenerationDispatchContracts: []config.NativeGenerationDispatchContract{valid}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := missingVocab(checks.PS6128, &tc.cfg); len(got) != tc.want {
				t.Errorf("missingVocab=%v, want count %d", got, tc.want)
			}
		})
	}
}
