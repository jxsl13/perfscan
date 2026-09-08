package checks

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6115(t *testing.T) {
	t.Parallel()
	results := analysistest.Run(t, analysistest.TestData(), ps6115TestAnalyzer(ps6115TestContracts()), "ps6115")
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			if len(diagnostic.SuggestedFixes) != 0 {
				t.Fatalf("PS6115 diagnostic unexpectedly has %d suggested fixes", len(diagnostic.SuggestedFixes))
			}
		}
	}
}

func TestPS6115ContractOrderIndependent(t *testing.T) {
	t.Parallel()
	contracts := ps6115TestContracts()
	slices.Reverse(contracts)
	analysistest.Run(t, analysistest.TestData(), ps6115TestAnalyzer(contracts), "ps6115")
}

func TestPS6115SilentWithoutConfig(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6115TestAnalyzer(nil), "ps6115silent")
}

func TestPS6115RejectsPackageAliasNamedC(t *testing.T) {
	t.Parallel()
	contract := ps6115Contract("fake-alias", "C.Forward")
	contract.Calls[0].ConfiguredSite = "ps6115fakecalias.fakeAlias"
	contract.Calls[0].HostAlternativeCallable = "ps6115fakecalias.hostForward"
	contract.Calls[0].OperationConstant = "ps6115fakecalias.opLoss"
	analysistest.Run(t, analysistest.TestData(), ps6115TestAnalyzer([]config.TinySynchronousAcceleratorScreenContract{contract}), "ps6115fakecalias")
}

func TestPS6115AmbiguousContractsStaySilent(t *testing.T) {
	t.Parallel()
	contract := ps6115Contract("literalGeometry", "ps6115.Metal.Forward")
	duplicateName := contract
	duplicateName.Calls = slices.Clone(contract.Calls)
	duplicateName.Calls[0].ConfiguredSite = "ps6115.constantGeometry"
	duplicateClaim := contract
	duplicateClaim.Name = "other"
	duplicateClaim.Calls = slices.Clone(contract.Calls)
	if got := ps6115Contracts([]config.TinySynchronousAcceleratorScreenContract{contract, duplicateName, duplicateClaim}); len(got) != 0 {
		t.Fatalf("ambiguous contracts produced %d usable contracts", len(got))
	}
}

func TestPS6115FixtureContractsAreComplete(t *testing.T) {
	t.Parallel()
	contracts := ps6115TestContracts()
	for index := range contracts {
		if !contracts[index].Valid() {
			t.Errorf("contract %q is invalid", contracts[index].Name)
		}
	}
	if got := len(ps6115Contracts(contracts)); got != len(contracts) {
		t.Fatalf("%d complete contracts reduced to %d unambiguous contracts", len(contracts), got)
	}
}

func TestPS6115OwnerEvidenceHonesty(t *testing.T) {
	t.Parallel()
	evidence := strings.Join(strings.Fields(PS6115.Doc.MeasuredWin), " ")
	for _, required := range []string{
		"about 120x", "median about 1.041x", "1.05x median", "1.03x every pair",
		"a0e7a529", "921abe97", "fc44e371", "no application win or routing conclusion",
	} {
		if !strings.Contains(evidence, required) {
			t.Errorf("owner evidence is missing %q", required)
		}
	}
	text := strings.Join(strings.Fields(PS6115.Doc.Text), " ")
	for _, required := range []string{"representative evidence, not runtime proof", "paired end-to-end validation", "There is NO automatic fix"} {
		if !strings.Contains(text, required) {
			t.Errorf("documentation honesty boundary is missing %q", required)
		}
	}
	if PS6115.AutoFix || !PS6115.NeedsConfig || PS6115.Level != 3 || PS6115.Category != "verify" ||
		len(PS6115.Vocab) != 1 || PS6115.Vocab[0] != "tinySynchronousAcceleratorScreenContracts" {
		t.Fatalf("PS6115 metadata drift: AutoFix=%v NeedsConfig=%v Level=%d Category=%q Vocab=%v",
			PS6115.AutoFix, PS6115.NeedsConfig, PS6115.Level, PS6115.Category, PS6115.Vocab)
	}
}

func ps6115TestAnalyzer(contracts []config.TinySynchronousAcceleratorScreenContract) *analysis.Analyzer {
	analyzer := *PS6115.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6115WithContracts(pass, contracts)
	}
	return &analyzer
}

func ps6115TestContracts() []config.TinySynchronousAcceleratorScreenContract {
	contracts := []config.TinySynchronousAcceleratorScreenContract{
		ps6115Contract("literalGeometry", "ps6115.Metal.Forward"),
		ps6115Contract("constantGeometry", "ps6115.Metal.Forward"),
		ps6115Contract("stableSnapshots", "ps6115.Metal.Forward"),
	}
	pair := ps6115Contract("completeForwardBackward", "ps6115.Metal.Forward")
	pair.Name = "complete-forward-gradient"
	pair.Calls = append(pair.Calls, config.TinySynchronousAcceleratorScreenCall{
		ConfiguredSite: "ps6115.completeForwardBackward", AcceleratorCallable: "ps6115.Metal.Backward",
		HostAlternativeCallable: "ps6115.hostBackward", RowsArgument: 2, ColumnsArgument: 3,
	})
	pair.ConfiguredSubmissionCount = 2
	contracts = append(contracts, pair)

	for _, site := range []string{
		"fakeCallee", "wrongOperation", "wrongOperationIdentity", "wrongRoles", "zeroGeometry", "negativeGeometry", "wideConstant",
		"dynamicGeometry", "mutatedRoot", "addressedRoot", "capturedRoot", "opaqueRoot", "interfaceCall",
		"functionValue", "callbackCall", "goCall", "deferCall", "genericCall", "variadicCall", "methodExpression",
		"wrapperCall", "nestedWrapper", "nestedDeviceWrapper", "duplicateCall", "visibleTransfer", "deviceResident", "graphContext", "laterDeviceConsumer",
		"selectorLaterDeviceConsumer", "indexLaterDeviceConsumer", "aliasLaterDeviceConsumer", "globalAliasEscape",
		"deviceBufferContext", "recorderContext", "streamContext", "commandBufferContext", "narrowingConversion",
		"uninitializedRoot", "promotedMethod", "genericSite", "unresolvedHost", "unreachableCall", "fakeCShape", "shadowedMethod", "missingCall",
	} {
		callable := "ps6115.Metal.Forward"
		switch site {
		case "interfaceCall":
			callable = "ps6115.Accelerator.Forward"
		case "goCall", "deferCall":
			callable = "ps6115.Metal.Submit"
		case "genericCall":
			callable = "ps6115.genericForward"
		case "variadicCall":
			callable = "ps6115.variadicForward"
		case "wideConstant":
			callable = "ps6115.wideForward"
		case "deviceResident":
			callable = "ps6115.DeviceMetal.Forward"
		case "narrowingConversion":
			callable = "ps6115.NarrowMetal.Forward"
		case "fakeCShape":
			callable = "C.Forward"
		}
		contracts = append(contracts, ps6115Contract(site, callable))
	}
	for index := range contracts {
		if contracts[index].Name == "unresolvedHost" {
			contracts[index].Calls[0].HostAlternativeCallable = "ps6115.missingHostForward"
		}
	}
	existing := ps6115Contract("existingSelector", "ps6115.Metal.Forward")
	existing.ExistingMeasuredHostSelector = true
	contracts = append(contracts, existing)
	retained := ps6115Contract("profiledRetain", "ps6115.Metal.Forward")
	retained.IntentionalRetainedAcceleratorRoute = true
	contracts = append(contracts, retained)
	missingGroup := ps6115Contract("missingGroupedForward", "ps6115.Metal.Forward")
	missingGroup.Calls = append(missingGroup.Calls, config.TinySynchronousAcceleratorScreenCall{
		ConfiguredSite: "ps6115.missingGroupedBackward", AcceleratorCallable: "ps6115.Metal.Backward",
		HostAlternativeCallable: "ps6115.hostBackward", RowsArgument: 2, ColumnsArgument: 3,
	})
	missingGroup.ConfiguredSubmissionCount = 2
	contracts = append(contracts, missingGroup)
	return contracts
}

func ps6115Contract(site, callable string) config.TinySynchronousAcceleratorScreenContract {
	return config.TinySynchronousAcceleratorScreenContract{
		Name: site,
		Calls: []config.TinySynchronousAcceleratorScreenCall{{
			ConfiguredSite: "ps6115." + site, AcceleratorCallable: callable, HostAlternativeCallable: "ps6115.hostForward",
			RowsArgument: 2, ColumnsArgument: 3, OperationArgument: 1, OperationConstant: "ps6115.opLoss", OperationConstantValue: "7",
		}},
		ConfiguredRows: 8, ConfiguredColumns: 10, ConfiguredWorkingSetBytes: 640,
		MaxElements: 80, MaxWorkingSetBytes: 640, ConfiguredSubmissionCount: 1,
		TargetGOOS: "darwin", TargetGOARCH: "arm64", MemoryModel: "unified",
		ConfiguredEvidence: "owner-paired-screen",
		BlockingCompletion: true, HostAccessibleInputs: true, HostAccessibleOutput: true, NoTransferRequired: true,
		NoPreexistingDeviceResidency: true, NoPostDeviceResidency: true, NoGraphContext: true, NoRecorderContext: true,
		NoCommandBufferContext: true, ExactDTypeCoverage: true, ExactLayoutCoverage: true, ExactAttributesCoverage: true,
		ExactReductionCoverage: true, ForwardParity: true, GradientParity: true, FloatingPointParity: true, ErrorParity: true,
		PanicParity: true, MutationParity: true, AliasParity: true, OwnershipParity: true, RecorderParity: true,
		AutogradParity: true, BackendSelectionParity: true, PairedEndToEndValidationRequired: true,
	}
}
