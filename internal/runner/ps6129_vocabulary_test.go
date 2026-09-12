package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

func TestPS6129Vocabulary(t *testing.T) {
	t.Parallel()
	check := &lint.Check{ID: "PS6129", NeedsConfig: true, Vocab: []string{"denseRowGEMMFuncs", "causalZeroGEMMContracts"}}
	for _, tc := range []struct {
		name string
		cfg  config.Config
		want int
	}{
		{"empty", config.Config{}, 2},
		{"exact_gemm", config.Config{DenseRowGEMMFuncs: []string{"example.org/math.gemm"}}, 1},
		{"malformed_gemm", config.Config{DenseRowGEMMFuncs: []string{"gemm"}}, 2},
		{"unreviewed_causal_contract", config.Config{CausalZeroGEMMContracts: []config.CausalZeroGEMMContract{{OwnerSite: "example.org/math.backward"}}}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := missingVocab(check, &tc.cfg); len(got) != tc.want {
				t.Fatalf("missing=%v want count%d", got, tc.want)
			}
		})
	}
}
