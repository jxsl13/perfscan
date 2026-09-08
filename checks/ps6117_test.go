package checks

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6117(t *testing.T) {
	t.Parallel()
	results := analysistest.Run(t, analysistest.TestData(), ps6117TestAnalyzer(ps6117TestContracts()), "ps6117")
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			if len(diagnostic.SuggestedFixes) != 0 {
				t.Fatalf("PS6117 diagnostic unexpectedly has %d suggested fixes", len(diagnostic.SuggestedFixes))
			}
		}
	}
}

func TestPS6117SilentWithoutConfig(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6117TestAnalyzer(nil), "ps6117silent")
}

func TestPS6117ContractOrderIndependent(t *testing.T) {
	t.Parallel()
	contracts := ps6117TestContracts()
	slices.Reverse(contracts)
	analysistest.Run(t, analysistest.TestData(), ps6117TestAnalyzer(contracts), "ps6117")
}

func TestPS6117AmbiguousContractsStaySilent(t *testing.T) {
	t.Parallel()
	contract := ps6117Contract("fragmented")
	duplicateName := contract
	duplicateName.ObjectiveSite = "ps6117.partial"
	duplicateName.Calls = ps6117Calls("partial")
	duplicateSite := contract
	duplicateSite.Name = "other"
	if got := ps6117Contracts([]config.FragmentedAcceleratorObjectiveContract{contract, duplicateName, duplicateSite}); len(got) != 0 {
		t.Fatalf("ambiguous contracts produced %d usable contracts", len(got))
	}
}

func TestPS6117ReviewedRoutesStaySilent(t *testing.T) {
	t.Parallel()
	existing := ps6117Contract("fragmented")
	ps6117RetargetContract(&existing, "ps6117silent")
	existing.ExistingWholeObjectiveRoute = true
	analysistest.Run(t, analysistest.TestData(), ps6117TestAnalyzer([]config.FragmentedAcceleratorObjectiveContract{existing}), "ps6117silent")

	retained := ps6117Contract("fragmented")
	ps6117RetargetContract(&retained, "ps6117silent")
	retained.IntentionalRetainedFragmentedRoute = true
	analysistest.Run(t, analysistest.TestData(), ps6117TestAnalyzer([]config.FragmentedAcceleratorObjectiveContract{retained}), "ps6117silent")
}

func TestPS6117ContractValidation(t *testing.T) {
	t.Parallel()
	valid := ps6117Contract("fragmented")
	if !valid.Valid() {
		t.Fatal("complete fragmented-objective contract is invalid")
	}
	tests := []struct {
		name string
		edit func(*config.FragmentedAcceleratorObjectiveContract)
	}{
		{"too few calls", func(c *config.FragmentedAcceleratorObjectiveContract) { c.ConfiguredAcceleratorCallCount = 3 }},
		{"partial count", func(c *config.FragmentedAcceleratorObjectiveContract) { c.Calls[1].ExpectedOccurrences = 1 }},
		{"sync mismatch", func(c *config.FragmentedAcceleratorObjectiveContract) { c.ConfiguredSynchronousBoundaryCount = 4 }},
		{"multiple candidate submits", func(c *config.FragmentedAcceleratorObjectiveContract) { c.CandidateSubmissionCount = 2 }},
		{"missing geometry", func(c *config.FragmentedAcceleratorObjectiveContract) { c.GeometryCacheKey = "" }},
		{"overlapping result", func(c *config.FragmentedAcceleratorObjectiveContract) {
			c.ParameterGradientsResult = c.ScalarObjectiveResult
		}},
		{"dynamic dispatch unknown", func(c *config.FragmentedAcceleratorObjectiveContract) { c.NoDynamicDispatch = false }},
		{"mutation unknown", func(c *config.FragmentedAcceleratorObjectiveContract) { c.InputsAndParametersImmutable = false }},
		{"residency unknown", func(c *config.FragmentedAcceleratorObjectiveContract) { c.NoPreexistingDeviceResidency = false }},
		{"benchmark missing", func(c *config.FragmentedAcceleratorObjectiveContract) { c.PairedApplicationBenchmarkRequired = false }},
		{"numerics missing", func(c *config.FragmentedAcceleratorObjectiveContract) { c.NumericalValidationRequired = false }},
		{"per-operation routes missing", func(c *config.FragmentedAcceleratorObjectiveContract) {
			c.PerOperationBackendRoutePreservationRequired = false
		}},
		{"true causal mask missing", func(c *config.FragmentedAcceleratorObjectiveContract) { c.TrueCausalMaskSemanticsRequired = false }},
		{"scatter assumption", func(c *config.FragmentedAcceleratorObjectiveContract) { c.ScatterNDNotAssumedFaster = false }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			candidate.Calls = slices.Clone(valid.Calls)
			test.edit(&candidate)
			if candidate.Valid() {
				t.Fatal("incomplete contract is valid")
			}
		})
	}
}

func TestPS6117OwnerEvidenceHonesty(t *testing.T) {
	t.Parallel()
	evidence := strings.Join(strings.Fields(PS6117.Doc.MeasuredWin), " ")
	for _, required := range []string{
		"ba513def", "#1201", "fab7e6e5", "m2-metal-gpt-loss-grad-20260824", "21", "3.689x", "2.822x",
		"20.748", "76.238", "245", "1,650", "26.3-31.6", "24.7-25.6", "scatterND", "one-hot",
		"discussion_r3840936867", "discussion_r3840936874", "per-operation backend routes", "finite -1e30 causal-mask leakage",
	} {
		if !strings.Contains(evidence, required) {
			t.Errorf("PS6117 measured evidence lost %q", required)
		}
	}
	text := strings.Join(strings.Fields(PS6117.Doc.Text), " ")
	for _, required := range []string{
		"broader than an exact forward -> loss -> backward", "fails closed", "NO automatic fix", "does not claim a win",
		"paired, same-binary application benchmarks", "scalar and every gradient", "shape-sensitive", "Never recommend scatterND",
		"per-operation backend route", "finite sentinel such as -1e30", "negative infinity", "true masked-softmax",
		"selected struct field or container index", "accelerator-dependent aggregate members",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("PS6117 documentation lost %q", required)
		}
	}
	if PS6117.AutoFix || !PS6117.NeedsConfig || PS6117.Level != 3 || PS6117.Category != "verify" ||
		len(PS6117.Vocab) != 1 || PS6117.Vocab[0] != "fragmentedAcceleratorObjectiveContracts" {
		t.Fatalf("PS6117 metadata drift: AutoFix=%v NeedsConfig=%v Level=%d Category=%q Vocab=%v", PS6117.AutoFix, PS6117.NeedsConfig, PS6117.Level, PS6117.Category, PS6117.Vocab)
	}
}

func ps6117TestAnalyzer(contracts []config.FragmentedAcceleratorObjectiveContract) *analysis.Analyzer {
	analyzer := *PS6117.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) { return runPS6117WithContracts(pass, contracts) }
	return &analyzer
}

func ps6117TestContracts() []config.FragmentedAcceleratorObjectiveContract {
	contracts := []config.FragmentedAcceleratorObjectiveContract{ps6117Contract("fragmented")}
	helper := ps6117Contract("fragmentedHelpers")
	helper.Name = "helper-objective"
	helper.ConfiguredAcceleratorCallCount = 4
	helper.ConfiguredSynchronousBoundaryCount = 4
	helper.Calls = []config.FragmentedAcceleratorObjectiveCall{
		{ConfiguredSite: "ps6117.helperForward", AcceleratorCallable: "ps6117.Backend.Execute", OperationArgument: 1, OperationConstant: "ps6117.opEmbed", OperationConstantValue: "1", ExpectedOccurrences: 1},
		{ConfiguredSite: "ps6117.helperForward", AcceleratorCallable: "ps6117.Backend.Execute", OperationArgument: 1, OperationConstant: "ps6117.opForward", OperationConstantValue: "2", ExpectedOccurrences: 1},
		{ConfiguredSite: "ps6117.fragmentedHelpers", AcceleratorCallable: "ps6117.Backend.Execute", OperationArgument: 1, OperationConstant: "ps6117.opLoss", OperationConstantValue: "3", ExpectedOccurrences: 1},
		{ConfiguredSite: "ps6117.fragmentedHelpers", AcceleratorCallable: "ps6117.Backend.Gradient", OperationArgument: 1, OperationConstant: "ps6117.opBackward", OperationConstantValue: "4", ExpectedOccurrences: 1},
	}
	contracts = append(contracts, helper)
	for _, site := range []string{"partial", "wrongOperation", "functionValue", "customHook", "mutation", "opaqueMutation", "deviceResident", "unreachableCall", "existingFusedCall", "closureCall", "genericObjective", "wrongResults", "discardedResults", "exclusiveBranches", "selectedStructField", "selectedContainerIndex"} {
		contract := ps6117Contract(site)
		contract.Name = site
		contracts = append(contracts, contract)
	}
	dynamic := ps6117Contract("dynamicDispatch")
	dynamic.Name = "dynamic-dispatch"
	for index := range dynamic.Calls {
		dynamic.Calls[index].AcceleratorCallable = strings.Replace(dynamic.Calls[index].AcceleratorCallable, "Backend", "DynamicBackend", 1)
	}
	contracts = append(contracts, dynamic)
	return contracts
}

func ps6117Calls(site string) []config.FragmentedAcceleratorObjectiveCall {
	prefix := "ps6117." + site
	return []config.FragmentedAcceleratorObjectiveCall{
		{ConfiguredSite: prefix, AcceleratorCallable: "ps6117.Backend.Execute", OperationArgument: 1, OperationConstant: "ps6117.opEmbed", OperationConstantValue: "1", ExpectedOccurrences: 1},
		{ConfiguredSite: prefix, AcceleratorCallable: "ps6117.Backend.Execute", OperationArgument: 1, OperationConstant: "ps6117.opForward", OperationConstantValue: "2", ExpectedOccurrences: 2},
		{ConfiguredSite: prefix, AcceleratorCallable: "ps6117.Backend.Execute", OperationArgument: 1, OperationConstant: "ps6117.opLoss", OperationConstantValue: "3", ExpectedOccurrences: 1},
		{ConfiguredSite: prefix, AcceleratorCallable: "ps6117.Backend.Gradient", OperationArgument: 1, OperationConstant: "ps6117.opBackward", OperationConstantValue: "4", ExpectedOccurrences: 1},
	}
}

func ps6117Contract(site string) config.FragmentedAcceleratorObjectiveContract {
	return config.FragmentedAcceleratorObjectiveContract{
		Name: "gpt-objective", ObjectiveSite: "ps6117." + site, Calls: ps6117Calls(site),
		ConfiguredAcceleratorCallCount: 5, ConfiguredSynchronousBoundaryCount: 5, CandidateSubmissionCount: 1,
		ScalarObjectiveResult: 1, ParameterGradientsResult: 2, ErrorResult: 3,
		GeometryCacheKey:               "batch=1,seq=256,ctx=256,vocab=4096,dim=512,heads=8,hidden=2048,depth=6,f32-contiguous",
		ConfiguredEvidence:             "goai-ba513def-fab7e6e5-m2-metal-gpt-loss-grad-20260824",
		CompleteForwardLossReverseMode: true, StableObjectiveGeometry: true, GeometryCacheKeyComplete: true,
		CacheReusedAcrossObjectives: true, EachAcceleratorCallSubmits: true, EachAcceleratorCallSynchronizesBeforeReturn: true,
		CandidateReturnsScalarAndAllParameterGradients: true, NoOtherMaterializedResults: true,
		NoDynamicDispatch: true, NoCustomHooks: true, InputsAndParametersImmutable: true, OutputsDoNotAliasInputs: true,
		NoPreexistingDeviceResidency: true, NoPostObjectiveDeviceResidency: true, ExactDTypeLayoutAttributesReduction: true,
		FloatingPointParityRequired: true, ErrorPanicParityRequired: true, RecorderAutogradBackendParityRequired: true,
		PerOperationBackendRoutePreservationRequired: true, TrueCausalMaskSemanticsRequired: true,
		PairedApplicationBenchmarkRequired: true, NumericalValidationRequired: true,
		ShapeAwareEmbeddingGradientValidationRequired: true, RepeatedIndexGradientParityRequired: true, ScatterNDNotAssumedFaster: true,
	}
}

func ps6117RetargetContract(contract *config.FragmentedAcceleratorObjectiveContract, packagePath string) {
	contract.ObjectiveSite = packagePath + ".fragmented"
	for index := range contract.Calls {
		contract.Calls[index].ConfiguredSite = packagePath + ".fragmented"
		contract.Calls[index].AcceleratorCallable = strings.Replace(contract.Calls[index].AcceleratorCallable, "ps6117.", packagePath+".", 1)
		contract.Calls[index].OperationConstant = strings.Replace(contract.Calls[index].OperationConstant, "ps6117.", packagePath+".", 1)
	}
}
