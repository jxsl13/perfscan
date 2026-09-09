package checks

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6085(t *testing.T) {
	t.Parallel()
	results := analysistest.Run(t, analysistest.TestData(), ps6085TestAnalyzer([]config.StateExpandedLookupContract{
		ps6085TestContract("ps6085", "decodeRow", "grid"),
	}), "ps6085")
	ps6085RequireAdvisoryDiagnostics(t, results, 1)
	message := results[0].Diagnostics[0].Message
	for _, want := range []string{"131072 bytes (196608 bytes combined", "no automatic fix"} {
		if !strings.Contains(message, want) {
			t.Fatalf("PS6085 diagnostic %q is missing %q", message, want)
		}
	}
}

func TestPS6085SupportedForms(t *testing.T) {
	t.Parallel()
	contracts := []config.StateExpandedLookupContract{
		ps6085TestContract("ps6085forms", "direct", "directGrid"),
		ps6085TestContract("ps6085forms", "byValue", "valueGrid"),
		ps6085TestContract("ps6085forms", "float64Row", "float64Grid"),
		ps6085TestContract("ps6085forms", "counted", "countedGrid"),
		ps6085TestMethodContract("ps6085forms", "decoder", "methodRow", "methodGrid"),
	}
	results := analysistest.Run(t, analysistest.TestData(), ps6085TestAnalyzer(contracts), "ps6085forms")
	ps6085RequireAdvisoryDiagnostics(t, results, 5)
}

func TestPS6085SilentWithoutContract(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6085TestAnalyzer(nil), "ps6085silent")
}

func TestPS6085InvalidContractStaysSilent(t *testing.T) {
	t.Parallel()
	contract := ps6085TestContract("ps6085silent", "decodeRow", "grid")
	contract.HotPathEvidence = false
	analysistest.Run(t, analysistest.TestData(), ps6085TestAnalyzer([]config.StateExpandedLookupContract{contract}), "ps6085silent")
}

func TestPS6085DuplicateContractsStaySilent(t *testing.T) {
	t.Parallel()
	first := ps6085TestContract("ps6085silent", "decodeRow", "grid")
	second := first
	second.Name = "duplicate"
	for index, contracts := range [][]config.StateExpandedLookupContract{{first, second}, {second, first}} {
		index, contracts := index, contracts
		t.Run(fmt.Sprintf("order-%d", index), func(t *testing.T) {
			t.Parallel()
			analysistest.Run(t, analysistest.TestData(), ps6085TestAnalyzer(contracts), "ps6085silent")
		})
	}
}

func TestPS6085RejectsNearMisses(t *testing.T) {
	t.Parallel()
	sites := []struct {
		owner string
		table string
	}{
		{owner: "noMultiply", table: "noMultiplyGrid"},
		{owner: "wrongLane", table: "wrongLaneGrid"},
		{owner: "conditionalLane", table: "conditionalLaneGrid"},
		{owner: "duplicateSites", table: "duplicateSitesGrid"},
		{owner: "nonconstantState", table: "nonconstantStateGrid"},
		{owner: "stateInsideLane", table: "stateInsideLaneGrid"},
		{owner: "sideEffectIndex", table: "sideEffectIndexGrid"},
		{owner: "laneDependentRow", table: "laneDependentRowGrid"},
		{owner: "breakLoop", table: "breakLoopGrid"},
		{owner: "stateEscape", table: "stateEscapeGrid"},
		{owner: "laneDependentScale", table: "laneScaleGrid"},
		{owner: "mutableCandidate", table: "mutableGrid"},
		{owner: "escapedCandidate", table: "escapedGrid"},
		{owner: "rowCopyMutation", table: "rowCopyMutationGrid"},
		{owner: "rowCopyAddress", table: "rowCopyAddressGrid"},
		{owner: "methodMutationCandidate", table: "methodMutationGrid"},
		{owner: "methodValueCandidate", table: "methodValueGrid"},
		{owner: "aggregateCandidate", table: "aggregateGrid"},
		{owner: "interfaceCandidate", table: "interfaceGrid"},
		{owner: "laneMutation", table: "laneMutationGrid"},
		{owner: "rowIndexMutation", table: "rowIndexMutationGrid"},
		{owner: "scaleMutation", table: "scaleMutationGrid"},
		{owner: "laneCapture", table: "laneCaptureGrid"},
		{owner: "initReadCandidate", table: "initReadGrid"},
		{owner: "initCallCandidate", table: "initCallGrid"},
		{owner: "roundedState", table: "roundedStateGrid"},
		{owner: "rowIndexAddress", table: "rowIndexAddressGrid"},
		{owner: "scaleAddress", table: "scaleAddressGrid"},
		{owner: "namedIndexMethod", table: "namedIndexGrid"},
		{owner: "namedScaleMethod", table: "namedScaleGrid"},
		{owner: "initIIFECandidate", table: "initIIFEGrid"},
		{owner: "initWrapperCandidate", table: "initWrapperGrid"},
		{owner: "initVarCandidate", table: "initVarGrid"},
		{owner: "initCallbackCandidate", table: "initCallbackGrid"},
		{owner: "unreachableCandidate", table: "unreachableGrid"},
	}
	contracts := make([]config.StateExpandedLookupContract, 0, len(sites))
	for _, site := range sites {
		contracts = append(contracts, ps6085TestContract("ps6085neg", site.owner, site.table))
	}
	analysistest.Run(t, analysistest.TestData(), ps6085TestAnalyzer(contracts), "ps6085neg")
}

func TestPS6085RejectsOpaqueInitCallback(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6085TestAnalyzer([]config.StateExpandedLookupContract{
		ps6085TestContract("ps6085opaque", "opaqueCandidate", "opaqueGrid"),
	}), "ps6085opaque")
}

func TestPS6085Documentation(t *testing.T) {
	t.Parallel()
	text := strings.Join(strings.Fields(PS6085.Doc.Text+"\n"+PS6085.Doc.MeasuredWin), " ")
	for _, want := range []string{
		"There is NO automatic fix",
		"initialization-order",
		"exact-output",
		"paired-benchmark",
		"cache",
		"ARM64 assembly",
		"Go fallback shape",
		"not a universal speedup",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("PS6085 documentation missing %q", want)
		}
	}
}

func ps6085TestAnalyzer(contracts []config.StateExpandedLookupContract) *analysis.Analyzer {
	analyzer := *PS6085.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6085WithContracts(pass, contracts)
	}
	return &analyzer
}

func ps6085RequireAdvisoryDiagnostics(t *testing.T, results []*analysistest.Result, want int) {
	t.Helper()
	diagnosticCount := 0
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			diagnosticCount++
			if len(diagnostic.SuggestedFixes) != 0 {
				t.Fatalf("PS6085 is advisory but returned fixes: %#v", diagnostic.SuggestedFixes)
			}
		}
	}
	if diagnosticCount != want {
		t.Fatalf("PS6085 reported %d diagnostics, want exactly %d", diagnosticCount, want)
	}
}

func ps6085TestContract(packagePath, owner, table string) config.StateExpandedLookupContract {
	return config.StateExpandedLookupContract{
		Name:                                  owner,
		OwnerSite:                             packagePath + "." + owner,
		OwnerKind:                             config.StateExpandedLookupFunction,
		TableObject:                           packagePath + "." + table,
		MaxStateCardinality:                   2,
		MaxExpandedBytes:                      128 << 10,
		HotPathEvidence:                       true,
		TableImmutableAfterInitialization:     true,
		InitializationReproducesExactDType:    true,
		InitializationOrderValidationRequired: true,
		CacheFootprintReviewRequired:          true,
		ExactOutputValidationRequired:         true,
		PairedBenchmarkRequired:               true,
	}
}

func ps6085TestMethodContract(packagePath, receiver, owner, table string) config.StateExpandedLookupContract {
	contract := ps6085TestContract(packagePath, owner, table)
	contract.OwnerSite = packagePath + "." + receiver + "." + owner
	contract.OwnerKind = config.StateExpandedLookupMethod
	return contract
}
