package runner

import (
	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/config"
	"testing"
)

func TestPS6141RunnerVocabulary(t *testing.T) {
	t.Parallel()
	c := config.SingleUseQuantizationContract{Quantizer: "fixture.pack", Consumer: "fixture.dot", ConsumerForm: "twoInputDot", PackedArgument: 0, WeightArgument: 1, RowsArgument: -1, QuantizationAndDotMeaningReviewed: true}
	for _, tc := range []struct {
		name      string
		contracts []config.SingleUseQuantizationContract
		missing   bool
	}{{"none", nil, true}, {"genuine", []config.SingleUseQuantizationContract{c}, false}, {"ambiguous", []config.SingleUseQuantizationContract{c, c}, true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Config{SingleUseQuantizationContracts: tc.contracts}
			missing := missingVocab(checks.PS6141, &cfg)
			if got := len(missing) == 1; got != tc.missing {
				t.Fatalf("missing=%v want=%v", missing, tc.missing)
			}
		})
	}
}
