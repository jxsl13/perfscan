package checks

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6118(t *testing.T) {
	t.Parallel()
	results := analysistest.Run(t, analysistest.TestData(), ps6118TestAnalyzer(ps6118TestContracts()), "ps6118")
	diagnostics := 0
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			diagnostics++
			if len(diagnostic.SuggestedFixes) != 0 {
				t.Fatalf("PS6118 diagnostic unexpectedly has %d suggested fixes", len(diagnostic.SuggestedFixes))
			}
			if len(diagnostic.Related) != 2 {
				t.Fatalf("PS6118 diagnostic has %d related locations, want optimizer and scalar observer", len(diagnostic.Related))
			}
		}
	}
	if diagnostics != 3 {
		t.Fatalf("PS6118 emitted %d diagnostics, want exactly three configured safe chains", diagnostics)
	}
}

func TestPS6118ContractOrderIndependent(t *testing.T) {
	t.Parallel()
	contracts := ps6118TestContracts()
	slices.Reverse(contracts)
	analysistest.Run(t, analysistest.TestData(), ps6118TestAnalyzer(contracts), "ps6118")
}

func TestPS6118FaithfulGoAIOwnerReplay(t *testing.T) {
	t.Parallel()
	contract := ps6118Contract("train")
	contract.Name = "goai-gpt-adamf32"
	contract.ConfiguredSite = "ps6118owner.train"
	contract.ObjectiveCallable = "ps6118owner.GPT.LossAndGrad"
	contract.OptimizerCallable = "ps6118owner.AdamF32.Step"
	contract.ScalarObserverCallable = "ps6118owner.observe"
	analysistest.Run(t, analysistest.TestData(), ps6118TestAnalyzer([]config.CrossStepAcceleratorResidencyContract{contract}), "ps6118owner")
}

func TestPS6118SilentWithoutConfig(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6118TestAnalyzer(nil), "ps6118silent")
}

func TestPS6118AmbiguousContractsStaySilent(t *testing.T) {
	t.Parallel()
	contract := ps6118Contract("fixed")
	duplicateName := ps6118Contract("secondFixed")
	duplicateName.Name = contract.Name
	duplicateClaim := contract
	duplicateClaim.Name = "another-name"
	if got := ps6118Contracts([]config.CrossStepAcceleratorResidencyContract{contract, duplicateName, duplicateClaim}); len(got) != 0 {
		t.Fatalf("ambiguous PS6118 contracts accepted: %+v", got)
	}
}

func TestPS6118OwnerEvidenceAndMetadata(t *testing.T) {
	t.Parallel()
	evidence := strings.Join(strings.Fields(PS6118.Doc.MeasuredWin), " ")
	for _, fragment := range []string{
		"PR #1202", "b86d9a755090df93ed64a30583774f30eefa0c3a", "20.75 ms", "1.91 ms",
		"4.56 ms", "10.5 ms", "15.529600 ms", "28.190119 ms", "1.81890x", "1.78324x",
	} {
		if !strings.Contains(evidence, fragment) {
			t.Errorf("PS6118 owner evidence missing %q", fragment)
		}
	}
	documentation := strings.Join(strings.Fields(PS6118.Doc.Text), " ")
	for _, fragment := range []string{
		"NO automatic fix", "explicit Sync/checkpoint", "does not claim a win", "receiver-owned parameter roles",
		"exact gradient callback", "returned-error control flow", "statically scalar-like host metric",
		"concurrent session access", "autograd", "backend-selection parity",
	} {
		if !strings.Contains(documentation, fragment) {
			t.Errorf("PS6118 documentation missing safety statement %q", fragment)
		}
	}
	if PS6118.AutoFix || !PS6118.NeedsConfig || PS6118.Level != 3 || PS6118.Category != "verify" ||
		len(PS6118.Vocab) != 1 || PS6118.Vocab[0] != "crossStepAcceleratorResidencyContracts" {
		t.Fatalf("PS6118 metadata drift: AutoFix=%v NeedsConfig=%v Level=%d Category=%q Vocab=%v",
			PS6118.AutoFix, PS6118.NeedsConfig, PS6118.Level, PS6118.Category, PS6118.Vocab)
	}
}

func ps6118TestAnalyzer(contracts []config.CrossStepAcceleratorResidencyContract) *analysis.Analyzer {
	analyzer := *PS6118.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6118WithContracts(pass, contracts)
	}
	return &analyzer
}

func ps6118TestContracts() []config.CrossStepAcceleratorResidencyContract {
	positive := []string{"fixed", "secondFixed", "faithfulOwnerReplay"}
	negative := []string{
		"wrongCount", "extraGradientConsumer", "extraScalarConsumer", "aliasGradient", "wrongCallback",
		"uncheckedObjectiveError", "ignoredOptimizerError",
	}
	contracts := make([]config.CrossStepAcceleratorResidencyContract, 0, len(positive)+len(negative)+4)
	for _, site := range append(positive, negative...) {
		contracts = append(contracts, ps6118Contract(site))
	}
	sliceScalar := ps6118Contract("sliceScalar")
	sliceScalar.ObjectiveCallable = "ps6118.SliceModel.LossAndGrad"
	sliceScalar.ScalarObserverCallable = "ps6118.observeSlice"
	contracts = append(contracts, sliceScalar)
	dynamic := ps6118Contract("dynamicObjective")
	dynamic.ObjectiveCallable = "ps6118.Objective.LossAndGrad"
	contracts = append(contracts, dynamic)
	existing := ps6118Contract("existingSession")
	existing.ExistingResidentSession = true
	contracts = append(contracts, existing)
	intentional := ps6118Contract("intentionalHost")
	intentional.IntentionalHostOptimizer = true
	contracts = append(contracts, intentional)
	return contracts
}

func ps6118Contract(site string) config.CrossStepAcceleratorResidencyContract {
	return config.CrossStepAcceleratorResidencyContract{
		Name: "training-" + site, ConfiguredSite: "ps6118." + site,
		ObjectiveCallable: "ps6118.Model.LossAndGrad", OptimizerCallable: "ps6118.Optimizer.Step",
		ScalarObserverCallable: "ps6118.observe", ObjectiveScalarResult: 1, ObjectiveGradientResult: 2,
		ObjectiveErrorResult: 3, OptimizerGradientCallbackArgument: 1, OptimizerErrorResult: 1,
		ScalarObserverArgument: 1, ObjectiveErrorControlFlow: "return", OptimizerErrorControlFlow: "return",
		ConfiguredLoopIterations: 4, ConfiguredParameterBytes: 4096,
		ConfiguredGradientBytes: 4096, ConfiguredAvoidableBytes: 8192, TargetGOOS: "darwin", TargetGOARCH: "arm64",
		MemoryModel: "unified", ConfiguredEvidence: "goai-pr1202-b86d9a755090df93ed64a30583774f30eefa0c3a",
		ObjectiveReceiverOwnsParameters: true, OptimizerReceiverOwnsParameters: true,
		ReceiverParameterSetsIdentical: true, GradientCallbackMapsReceiverParameter: true,
		DenseGradientPerParameter: true, StableParameterGradientOrder: true, ParameterUploadEveryStep: true,
		GradientHostMaterializedEveryStep: true, HostOptimizerEveryStep: true, ParametersReuploadedNextStep: true,
		OnlyScalarObservedBetweenSteps: true, ScalarResultIsHostMetric: true, ScalarObservationDoesNotSynchronizeState: true,
		NoIntermediateCheckpointRequired: true, ObjectiveAndOptimizerSynchronous: true,
		ObjectiveDoesNotRetainArguments: true, OptimizerDoesNotRetainArguments: true, NoCustomHooks: true,
		NoAliasesOrEscapes: true, NoConcurrentSessionAccess: true, DeviceStorageAndTransfersKnown: true,
		ResidentSessionConstructionSerialized: true,
		ResidentSessionOpportunityConfirmed:   true, ExactDTypeCoverage: true, ExactLayoutCoverage: true,
		ExactOptimizerCoverage: true, LifetimeParity: true, NumericalParity: true, CheckpointParity: true,
		ErrorAndPanicParity: true, MutationParity: true, OwnershipParity: true, AutogradParity: true,
		BackendSelectionParity: true, ExplicitSyncRequired: true, ExplicitCheckpointRequired: true,
		PairedEndToEndValidationRequired: true,
	}
}
