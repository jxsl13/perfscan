package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/perfscan/config"
)

func TestPS6054(t *testing.T) {
	defer config.SetForTesting(config.Config{SelectorPromotionSymbols: []string{
		"selectAttention", "selectCapability", "selectCorrectness", "selectUnbenchmarked",
	}})()
	analysistest.Run(t, analysistest.TestData(), PS6054.Analyzer, "ps6054")
}

func TestPS6054CompleteGate(t *testing.T) {
	defer config.SetForTesting(config.Config{SelectorPromotionSymbols: []string{
		"selectAttention=>BenchmarkTinyLlamaDecode|samples=10|independent|alternating|equivalent-outputs|retained-shapes<=0.5%",
	}})()
	analysistest.Run(t, analysistest.TestData(), PS6054.Analyzer, "ps6054gate")
}

func TestPS6054SilentWithoutVocabulary(t *testing.T) {
	defer config.SetForTesting(config.Config{})()
	analysistest.Run(t, analysistest.TestData(), PS6054.Analyzer, "ps6054silent")
}

func TestPS6054PromotionGateParser(t *testing.T) {
	selectors, gates := ps6054Config(map[string]bool{
		"selectAttention": true,
		"selectAttention=>BenchmarkDecode|samples=10|independent|alternating|equivalent-outputs|retained-shapes<=0.5%": true,
	})
	if !selectors["selectAttention"] {
		t.Fatal("selector name was not preserved")
	}
	if missing := gates["selectAttention"].missing(); len(missing) != 0 {
		t.Fatalf("complete promotion gate is missing %v", missing)
	}

	_, incomplete := ps6054Config(map[string]bool{
		"selectAttention=>BenchmarkDecode|samples=4|alternating": true,
	})
	missing := incomplete["selectAttention"].missing()
	if len(missing) != 3 {
		t.Fatalf("incomplete promotion gate missing = %v, want three axes", missing)
	}
}
