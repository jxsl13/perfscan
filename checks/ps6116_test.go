package checks

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6116(t *testing.T) {
	t.Parallel()
	results := analysistest.Run(t, analysistest.TestData(), ps6116TestAnalyzer(ps6116TestContracts()), "ps6116")
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			if len(diagnostic.SuggestedFixes) != 0 {
				t.Fatalf("PS6116 diagnostic unexpectedly has %d suggested fixes", len(diagnostic.SuggestedFixes))
			}
		}
	}
}

func TestPS6116ContractOrderIndependent(t *testing.T) {
	t.Parallel()
	contracts := ps6116TestContracts()
	slices.Reverse(contracts)
	analysistest.Run(t, analysistest.TestData(), ps6116TestAnalyzer(contracts), "ps6116")
}

func TestPS6116SilentWithoutConfig(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6116TestAnalyzer(nil), "ps6116silent")
}

func TestPS6116AuthoritativeOwnerReplay(t *testing.T) {
	t.Parallel()
	contracts := []config.ForwardLossBackwardGraphContract{ps6116OwnerContract("LossAndGrad", "goai-vit-owner")}
	for _, site := range []string{"LossAndGradTapeOption", "LossAndGradCEOption", "LossAndGradExpandedCEOptions"} {
		contracts = append(contracts, ps6116OwnerContract(site, site))
	}
	analysistest.Run(t, analysistest.TestData(), ps6116TestAnalyzer(contracts), "ps6116owner")
}

func ps6116OwnerContract(site, name string) config.ForwardLossBackwardGraphContract {
	contract := ps6116Contract(site)
	contract.Name = name
	contract.ObjectiveSite = "ps6116owner.ViT." + site
	contract.RecorderFactoryCallable = "ps6116owner.NewTapeOn"
	contract.RecorderBindingCallable = "ps6116owner.Context.WithRecorder"
	contract.ForwardCallable = "ps6116owner.ViT.Forward"
	contract.LossCallable = "ps6116owner.CrossEntropy"
	contract.BackwardCallable = "ps6116owner.Tape.Backward"
	contract.ParameterOrderCallable = "ps6116owner.ViT.Params"
	contract.GradientCallable = "ps6116owner.Tape.Grad"
	contract.LossReductionArgument = 0
	contract.LossReductionConstant = ""
	contract.LossReductionConstantValue = ""
	contract.ConfiguredGeometry = "F32 B=8,S=65,D=128,H=4,FFN=512,depth=4,C=10"
	return contract
}

func TestPS6116AmbiguousContractsStaySilent(t *testing.T) {
	t.Parallel()
	contract := ps6116Contract("objective")
	duplicateName := contract
	duplicateName.ObjectiveSite = "ps6116.wrongReduction"
	duplicateSite := contract
	duplicateSite.Name = "other"
	if got := ps6116Contracts([]config.ForwardLossBackwardGraphContract{contract, duplicateName, duplicateSite}); len(got) != 0 {
		t.Fatalf("ambiguous contracts produced %d usable contracts", len(got))
	}
}

func TestPS6116InvalidSemanticContractsStaySilent(t *testing.T) {
	t.Parallel()
	base := ps6116Contract("objective")
	tests := []struct {
		name   string
		mutate func(*config.ForwardLossBackwardGraphContract)
	}{
		{"custom hooks not excluded", func(c *config.ForwardLossBackwardGraphContract) { c.CustomHooksExcluded = false }},
		{"mutation not excluded", func(c *config.ForwardLossBackwardGraphContract) { c.MutationExcluded = false }},
		{"recorder not isolated", func(c *config.ForwardLossBackwardGraphContract) { c.RecorderIsolation = false }},
		{"per-operation routes present", func(c *config.ForwardLossBackwardGraphContract) { c.PerOperationAndLayerRoutesExcluded = false }},
		{"private-tape routes lost", func(c *config.ForwardLossBackwardGraphContract) { c.PrivateTapePreservesBackendRoutes = false }},
		{"backend selection differs", func(c *config.ForwardLossBackwardGraphContract) { c.BackendSelectionParity = false }},
		{"fallback absent", func(c *config.ForwardLossBackwardGraphContract) { c.PortableFallbackPreserved = false }},
		{"unbounded cache", func(c *config.ForwardLossBackwardGraphContract) { c.MaxCacheEntries = 0 }},
		{"multiple candidate submissions", func(c *config.ForwardLossBackwardGraphContract) { c.ConfiguredCandidateSubmissions = 2 }},
		{"numerical proof absent", func(c *config.ForwardLossBackwardGraphContract) { c.PairedNumericalValidationRequired = false }},
		{"end-to-end proof absent", func(c *config.ForwardLossBackwardGraphContract) { c.PairedEndToEndValidationRequired = false }},
	}
	for _, test := range tests {
		contract := base
		test.mutate(&contract)
		if contract.Valid() {
			t.Errorf("%s contract unexpectedly valid", test.name)
		}
	}
}

func TestPS6116OwnerEvidenceHonesty(t *testing.T) {
	t.Parallel()
	evidence := strings.Join(strings.Fields(PS6116.Doc.MeasuredWin), " ")
	for _, required := range []string{
		"d99bc4b5f02a80e03db7c29684efd61207143e41",
		"686f27f0a16d8f56526849ac87bda6d97fea029e",
		"overall 21-sample median was 1.456x", "minimum was 1.321x", "all 56 parameter gradients",
		"not a generic fusion or application-win claim", "3840658100", "3840658103",
		"bypass per-operation/layer routes", "discard those routes during backward",
	} {
		if !strings.Contains(evidence, required) {
			t.Errorf("owner evidence is missing %q", required)
		}
	}
	text := strings.Join(strings.Fields(PS6116.Doc.Text), " ")
	for _, required := range []string{
		"portable private-tape path authoritative", "paired numerical proof", "paired end-to-end campaigns",
		"There is NO automatic fix", "does not claim graph fusion",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("documentation honesty boundary is missing %q", required)
		}
	}
	if PS6116.AutoFix || !PS6116.NeedsConfig || PS6116.Level != 3 || PS6116.Category != "verify" ||
		len(PS6116.Vocab) != 1 || PS6116.Vocab[0] != "forwardLossBackwardGraphContracts" {
		t.Fatalf("PS6116 metadata drift: AutoFix=%v NeedsConfig=%v Level=%d Category=%q Vocab=%v",
			PS6116.AutoFix, PS6116.NeedsConfig, PS6116.Level, PS6116.Category, PS6116.Vocab)
	}
}

func ps6116TestAnalyzer(contracts []config.ForwardLossBackwardGraphContract) *analysis.Analyzer {
	analyzer := *PS6116.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6116WithContracts(pass, contracts)
	}
	return &analyzer
}

func ps6116TestContracts() []config.ForwardLossBackwardGraphContract {
	contracts := []config.ForwardLossBackwardGraphContract{ps6116Contract("objective")}
	for _, site := range []string{
		"routedContext", "mismatchedRecorderContext", "wrappedForwardInput", "mutatingForwardInput",
		"gradientLoopHook", "gradientLoopSideEffect",
		"wrongReduction", "extraForwardUse", "aliasForward", "duplicateForward", "wrappedForward",
		"dynamicForward", "functionValueForward", "recorderMutation", "customHook", "reversedGradientOrder",
		"partialGradientOrder", "nestedPartialGradientOrder", "guardSideEffect", "malformedBackwardGuard",
		"conditionalChain", "unreachableChain", "promotedForward", "genericObjective",
	} {
		contracts = append(contracts, ps6116Contract(site))
	}
	return contracts
}

func ps6116Contract(site string) config.ForwardLossBackwardGraphContract {
	name := site
	if site == "objective" {
		name = "vit-m2"
	}
	return config.ForwardLossBackwardGraphContract{
		Name: name, ObjectiveSite: "ps6116." + site,
		RecorderFactoryCallable: "ps6116.NewTape", RecorderBindingCallable: "ps6116.Context.WithRecorder",
		ForwardCallable: "ps6116.Model.Forward", LossCallable: "ps6116.CrossEntropy",
		BackwardCallable: "ps6116.Tape.Backward", ParameterOrderCallable: "ps6116.Model.Params",
		GradientCallable: "ps6116.Tape.Grad", RecorderFactoryBackendArgument: 1,
		RecorderBindingArgument: 1, ForwardRecorderArgument: 1,
		LossRecorderArgument: 1, LossForwardArgument: 2, BackwardLossArgument: 1, GradientParameterArgument: 1,
		LossReductionArgument: 4, LossReductionConstant: "ps6116.Mean", LossReductionConstantValue: "1",
		ConfiguredGeometry: "B=8,S=65,D=128,H=4,FFN=512,depth=4,C=10", ConfiguredGradientCount: 56,
		ConfiguredCurrentSubmissionCount: 3, ConfiguredCandidateSubmissions: 1, MaxCacheEntries: 4,
		ConfiguredEvidence:   "goai-pr1199-d99bc4b5-merge-686f27f",
		RecreatesForwardWork: true, StableGeometry: true, ExactScalarLossReduction: true,
		StableCompleteGradientOrder: true, PrivateRecorderOwnership: true, CustomHooksExcluded: true,
		MutationExcluded: true, DTypeLayoutBackendConstrained: true, CacheKeyCoversGeometry: true,
		CacheKeyCoversDTypeLayoutObjective: true, BoundedCache: true, PortableFallbackPreserved: true,
		ForwardParity: true, ScalarLossParity: true, AllGradientParity: true,
		InputAndParameterImmutability: true, ErrorAndPanicParity: true, RecorderIsolation: true,
		PerOperationAndLayerRoutesExcluded: true, PrivateTapePreservesBackendRoutes: true, BackendSelectionParity: true,
		PairedNumericalValidationRequired: true, PairedEndToEndValidationRequired: true,
	}
}
