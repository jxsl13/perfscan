package checks

import (
	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"strings"
	"testing"
)

func TestPS6141RegisteredAnalyzer(t *testing.T) {
	t.Parallel()
	c := config.SingleUseQuantizationContract{Quantizer: "ps6141.pack", Consumer: "ps6141.dot", ConsumerForm: "twoInputDot", PackedArgument: 0, WeightArgument: 1, RowsArgument: -1, QuantizationAndDotMeaningReviewed: true}
	analyzer := *PS6141.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6141WithContracts(pass, []config.SingleUseQuantizationContract{c})
	}
	results := analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6141")
	count := 0
	for _, result := range results {
		for _, d := range result.Diagnostics {
			count++
			if len(d.SuggestedFixes) != 0 || len(d.Related) != 1 {
				t.Fatalf("advisory fix/consumer contract=%+v", d)
			}
		}
	}
	if count != 1 {
		t.Fatalf("diagnostics=%d want1", count)
	}
}

func TestPS6141RegistryAndEvidence(t *testing.T) {
	t.Parallel()
	registered, ok := ByID("PS6141")
	if !ok || registered != PS6141 || PS6141.AutoFix || PS6141.Level != 3 || PS6141.Category != "verify" || !PS6141.NeedsConfig || len(PS6141.Vocab) != 1 || PS6141.Vocab[0] != "singleUseQuantizationContracts" {
		t.Fatalf("registry metadata=%+v", registered)
	}
	text := PS6141.Doc.Text + PS6141.Doc.MeasuredWin
	for _, fragment := range []string{"0.87x", "182032", "208481", "one extra allocation", "M=1", "N rows", "no automatic fix", "oracle", "not locally reproduced", "MIXED"} {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(fragment)) {
			t.Errorf("missing evidence/boundary %q", fragment)
		}
	}
}

func TestPS6141NoVocabulary(t *testing.T) {
	t.Parallel()
	pass, _ := ps6141TypedFixture(t, ps6141TwoInputSynthetic)
	count := 0
	pass.Report = func(analysis.Diagnostic) { count++ }
	if _, err := runPS6141WithContracts(pass, nil); err != nil || count != 0 {
		t.Fatalf("no vocabulary count=%d error=%v", count, err)
	}
}

func TestPS6141CompiledVocabularyEntryPoint(t *testing.T) {
	t.Parallel()
	c := ps6141TestContract()
	c.ConsumerForm = "twoInputDot"
	c.WeightArgument = 1
	c.RowsArgument = -1
	compiled := (&config.Config{SingleUseQuantizationContracts: []config.SingleUseQuantizationContract{c}}).Compile()
	pass, _ := ps6141TypedFixture(t, ps6141TwoInputSynthetic)
	calls, count := 0, 0
	pass.Report = func(analysis.Diagnostic) { count++ }
	if _, err := runPS6141WithVocabulary(pass, func() config.Sets { calls++; return compiled }); err != nil || calls != 1 || count != 1 {
		t.Fatalf("compiled current entrypoint calls=%d diagnostics=%d error=%v", calls, count, err)
	}
}
