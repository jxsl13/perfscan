package checks

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6111(t *testing.T) {
	t.Parallel()
	results := analysistest.Run(t, analysistest.TestData(), ps6111TestAnalyzer(ps6111TestContracts()), "ps6111", "ps6111neg", "ps6111fix", "ps6111consumer")
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			if len(diagnostic.SuggestedFixes) != 0 {
				t.Fatalf("PS6111 is advisory but returned fixes: %#v", diagnostic.SuggestedFixes)
			}
		}
	}
}

func TestPS6111SilentWithoutConfig(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6111TestAnalyzer(nil), "ps6111fix", "ps6111ambiguous")
}

func TestPS6111DuplicateWrapperContractsAreOrderIndependent(t *testing.T) {
	t.Parallel()
	first := ps6111Contract("ps6111ambiguous", "Step", "StepInto", nil)
	second := first
	second.Name = "second"
	second.ConfiguredResultElements = 16
	second.ConfiguredLoopIterations = 4
	for index, contracts := range [][]config.ReusableResultLoopContract{{first, second}, {second, first}} {
		t.Run(fmt.Sprintf("order-%d", index), func(t *testing.T) {
			t.Parallel()
			analysistest.Run(t, analysistest.TestData(), ps6111TestAnalyzer(contracts), "ps6111ambiguous")
		})
	}
}

func TestPS6111Documentation(t *testing.T) {
	t.Parallel()
	text := strings.Join(strings.Fields(PS6111.Doc.Text+"\n"+PS6111.Doc.MeasuredWin), " ")
	for _, want := range []string{
		"does not infer this relationship from Into, To, Append",
		"index used only as the same slice's element subscript",
		"feasible back edge after the call",
		"Fresh-identity observation",
		"partial or conditional traversal",
		"multi-slice TopKN/TopKNInto evidence",
		"There is NO automatic fix",
		"1,638,400 fewer bytes",
		"median 1.004x",
		"median 1.010x",
		"0.981x median primitive throughput",
		"pull requests 1214, 1215, and 1216",
		"eight wrapper allocations versus one destination allocation",
		"no local timing result is claimed",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("PS6111 documentation missing %q", want)
		}
	}
}

func ps6111TestAnalyzer(contracts []config.ReusableResultLoopContract) *analysis.Analyzer {
	analyzer := *PS6111.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6111WithContracts(pass, contracts)
	}
	return &analyzer
}

func ps6111TestContracts() []config.ReusableResultLoopContract {
	base := ps6111Contract("ps6111", "Step", "StepInto", nil)
	base.ConfiguredResultElements = 50257
	base.ConfiguredLoopIterations = 8
	sized := ps6111Contract("ps6111", "SizedStep", "SizedStepInto", []int{2})
	negative := ps6111Contract("ps6111neg", "Step", "StepInto", nil)
	negativeSized := ps6111Contract("ps6111neg", "SizedStep", "SizedStepInto", []int{2})
	interfaceContract := ps6111Contract("ps6111neg", "Dynamic.Step", "Dynamic.StepInto", nil)
	imported := ps6111Contract("ps6111backend", "Step", "StepInto", nil)
	return []config.ReusableResultLoopContract{base, sized, negative, negativeSized, interfaceContract, imported}
}

func ps6111Contract(packagePath, wrapper, into string, shape []int) config.ReusableResultLoopContract {
	if strings.Contains(wrapper, ".") {
		return ps6111ContractWithIDs(packagePath+"."+wrapper, packagePath+"."+into, shape)
	}
	return ps6111ContractWithIDs(packagePath+".Decoder."+wrapper, packagePath+".Decoder."+into, shape)
}

func ps6111ContractWithIDs(wrapper, into string, shape []int) config.ReusableResultLoopContract {
	return config.ReusableResultLoopContract{
		Name:                     wrapper,
		Wrapper:                  wrapper,
		Into:                     into,
		ResultPosition:           1,
		DestinationArgument:      3,
		ShapeArgumentPositions:   shape,
		WrapperReturnsFreshOwned: true,
		ResultLengthStableForReceiverAndListedArgs:     true,
		IntoOverwritesDestinationOnSuccess:             true,
		IntoDoesNotReadDestinationBeforeOverwrite:      true,
		IntoIgnoresDestinationIdentityAndExtraCapacity: true,
		IntoDoesNotRetainDestination:                   true,
		IntoExecutesSynchronously:                      true,
		WrapperAndIntoHaveIdenticalStateEffects:        true,
		WrapperAndIntoHaveIdenticalErrorsAndPanics:     true,
	}
}
