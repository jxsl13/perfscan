package checks

import (
	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"strings"
	"testing"
)

func TestPS6141AdvisoryIntegration(t *testing.T) {
	t.Parallel()
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "genuinePrerequisite", true: "conflictingContracts"}[duplicate], func(t *testing.T) {
			t.Parallel()
			pass, _ := ps6141TypedFixture(t, ps6141TwoInputSynthetic)
			c := ps6141TestContract()
			c.ConsumerForm = "twoInputDot"
			c.WeightArgument = 1
			c.RowsArgument = -1
			contracts := []config.SingleUseQuantizationContract{c}
			if duplicate {
				contracts = append(contracts, c)
			}
			var diagnostics []analysis.Diagnostic
			pass.Report = func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }
			if _, err := runPS6141WithContracts(pass, contracts); err != nil {
				t.Fatal(err)
			}
			want := 1
			if duplicate {
				want = 0
			}
			if len(diagnostics) != want {
				t.Fatalf("diagnostics=%d want=%d", len(diagnostics), want)
			}
			for _, d := range diagnostics {
				if len(d.SuggestedFixes) != 0 || len(d.Related) != 1 || !strings.Contains(d.Message, "benchmark the complete") || !strings.Contains(d.Message, "no numerical-equivalence") && !strings.Contains(d.Message, "numerical-equivalence claim") {
					t.Fatalf("advisory contract=%+v", d)
				}
			}
		})
	}
}
