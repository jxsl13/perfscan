package checks

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6120(t *testing.T) {
	t.Parallel()
	results := analysistest.Run(t, analysistest.TestData(), ps6120TestAnalyzer(ps6120TestContracts()), "ps6120")
	count := 0
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			count++
			if len(diagnostic.SuggestedFixes) != 0 {
				t.Fatal("PS6120 advisory supplied a fix")
			}
		}
	}
	if count != 9 {
		t.Fatalf("PS6120 diagnostics = %d, want 9", count)
	}
}

func TestPS6120SilentWithoutConfig(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6120TestAnalyzer(nil), "ps6120silent")
}

func TestPS6120MetadataAndEvidence(t *testing.T) {
	t.Parallel()
	text := strings.Join(strings.Fields(PS6120.Doc.Text+PS6120.Doc.MeasuredWin), " ")
	for _, fragment := range []string{"04838955aa12ce709c46f5a3cea22446534b7cb4", "1024 x 8", "2 KiB", "4.278x", "2.302x", "sync.Once", "native shader", "NO automatic fix", "host-transfer"} {
		if !strings.Contains(text, fragment) {
			t.Errorf("documentation missing %q", fragment)
		}
	}
	if PS6120.AutoFix || !PS6120.NeedsConfig || PS6120.Level != 3 || PS6120.Vocab[0] != "residentCodebookContracts" {
		t.Fatalf("metadata drift: %+v", PS6120)
	}
}

func TestPS6120ContractValidation(t *testing.T) {
	t.Parallel()
	valid := ps6120Contract("external", true)
	if !valid.Valid() {
		t.Fatal("valid contract rejected")
	}
	invalid := valid
	invalid.PairedResidentAndHostBenchmarkRequired = false
	if invalid.Valid() {
		t.Fatal("incomplete contract accepted")
	}
}

func TestPS6120AmbiguousContractsStaySilent(t *testing.T) {
	t.Parallel()
	base := ps6120Contract("external", true)
	duplicate := base
	duplicate.Name = "other"
	if got := ps6120Contracts([]config.ResidentCodebookContract{base, duplicate}); len(got) != 0 {
		t.Fatalf("ambiguous contracts accepted: %v", got)
	}
}

func ps6120TestAnalyzer(contracts []config.ResidentCodebookContract) *analysis.Analyzer {
	analyzer := *PS6120.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) { return runPS6120WithContracts(pass, contracts) }
	return &analyzer
}

func ps6120TestContracts() []config.ResidentCodebookContract {
	sites := []string{"literal", "external", "dynamic", "checked", "mapRange", "channelRange", "stringRange", "nested", "once", "actualOnce", "alias", "zero", "oneTrip", "unreachable", "deadNested", "indexMutation", "indexAddress", "discardedDispatchError", "scalarDevice", "interfaceDevice", "interfaceUploadParam", "interfaceDispatchParam", "wrongProducerGuard", "swallowedUploadError", "roundedConstants"}
	contracts := make([]config.ResidentCodebookContract, 0, len(sites))
	for _, site := range sites {
		contracts = append(contracts, ps6120Contract(site, site != "literal" && site != "roundedConstants"))
	}
	return contracts
}

func ps6120Contract(site string, external bool) config.ResidentCodebookContract {
	c := config.ResidentCodebookContract{Name: site, ConfiguredSite: "ps6120." + site, UploadCallable: "ps6120.Device.Upload", DispatchCallable: "ps6120.Device.Dispatch", UploadDataArgument: 1, DispatchBufferArgument: 1, WideElementBytes: 4, PackedBitsPerSymbol: 2, SymbolCount: 8, DistinctValueCount: 3, RowCount: 1, SymbolsPerPackedWord: 8, PackedWordBits: 16, MaxCodebookBytes: 2, ConfiguredEvidence: "reviewed-owner-evidence", NativeBoundaryEvidence: "reviewed-shader-binding-4", UploadCopiesSynchronously: true, UploadDoesNotRetainHostSlice: true, DeviceBufferImmutableDuringDispatch: true, HostPackingMatchesNativeDecode: true, NativeDecodeExactAndCheap: true, PairedResidentAndHostBenchmarkRequired: true, NumericalParityRequired: true, LifetimeAndSynchronizationValidationRequired: true}
	if external {
		c.ProducerCallable = "ps6120.exactCodebook"
		c.ProducerReturnsFreshWideCodebook = true
		c.ProducerExactAndInvariant = true
	}
	if site == "checked" {
		c.ProducerCallable = "ps6120.exactCodebookChecked"
		c.UploadCallable = "ps6120.CheckedDevice.Upload"
		c.DispatchCallable = "ps6120.CheckedDevice.Dispatch"
	}
	switch site {
	case "actualOnce":
		c.ConfiguredSite = "ps6120.actualOnceSite"
	case "indexMutation":
		c.UploadCallable, c.DispatchCallable = "ps6120.GuardDevice.Upload", "ps6120.GuardDevice.DispatchMutating"
	case "indexAddress":
		c.UploadCallable, c.DispatchCallable = "ps6120.GuardDevice.Upload", "ps6120.GuardDevice.DispatchAddress"
	case "discardedDispatchError":
		c.UploadCallable, c.DispatchCallable = "ps6120.ErrorDevice.Upload", "ps6120.ErrorDevice.Dispatch"
	case "scalarDevice":
		c.UploadCallable, c.DispatchCallable = "ps6120.ScalarDevice.Upload", "ps6120.ScalarDevice.Dispatch"
	case "interfaceDevice":
		c.UploadCallable, c.DispatchCallable = "ps6120.InterfaceDevice.Upload", "ps6120.InterfaceDevice.Dispatch"
	case "interfaceUploadParam":
		c.UploadCallable, c.DispatchCallable = "ps6120.InterfaceUploadDevice.Upload", "ps6120.InterfaceUploadDevice.Dispatch"
	case "interfaceDispatchParam":
		c.UploadCallable, c.DispatchCallable = "ps6120.InterfaceDispatchDevice.Upload", "ps6120.InterfaceDispatchDevice.Dispatch"
	case "roundedConstants":
		c.DistinctValueCount = 2
	case "wrongProducerGuard", "swallowedUploadError":
		c.ProducerCallable = "ps6120.exactCodebookChecked"
		c.UploadCallable = "ps6120.CheckedDevice.Upload"
		c.DispatchCallable = "ps6120.CheckedDevice.Dispatch"
	}
	return c
}
