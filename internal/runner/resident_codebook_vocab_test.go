package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

func TestMissingVocabResidentCodebookContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"residentCodebookContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 {
		t.Fatalf("empty missingVocab = %v", got)
	}
	invalid := config.Config{ResidentCodebookContracts: []config.ResidentCodebookContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 {
		t.Fatalf("invalid missingVocab = %v", got)
	}
	valid := config.ResidentCodebookContract{Name: "iq2", ConfiguredSite: "example.com/run", ProducerCallable: "example.com/grid", UploadCallable: "example.com/Device.Upload", DispatchCallable: "example.com/Device.Dispatch", UploadDataArgument: 1, DispatchBufferArgument: 1, WideElementBytes: 4, PackedBitsPerSymbol: 2, SymbolCount: 8, DistinctValueCount: 3, RowCount: 1, SymbolsPerPackedWord: 8, PackedWordBits: 16, MaxCodebookBytes: 2, ConfiguredEvidence: "hash", NativeBoundaryEvidence: "binding", ProducerReturnsFreshWideCodebook: true, ProducerExactAndInvariant: true, UploadCopiesSynchronously: true, UploadDoesNotRetainHostSlice: true, DeviceBufferImmutableDuringDispatch: true, HostPackingMatchesNativeDecode: true, NativeDecodeExactAndCheap: true, PairedResidentAndHostBenchmarkRequired: true, NumericalParityRequired: true, LifetimeAndSynchronizationValidationRequired: true}
	duplicateClaim := valid
	duplicateClaim.Name = "other"
	duplicates := config.Config{ResidentCodebookContracts: []config.ResidentCodebookContract{valid, duplicateClaim}}
	if got := missingVocab(check, &duplicates); len(got) != 1 {
		t.Fatalf("duplicate-claim missingVocab = %v", got)
	}
	duplicateName := valid
	duplicateName.ConfiguredSite = "example.com/otherRun"
	duplicateName.UploadCallable = "example.com/Other.Upload"
	duplicateName.DispatchCallable = "example.com/Other.Dispatch"
	duplicates = config.Config{ResidentCodebookContracts: []config.ResidentCodebookContract{valid, duplicateName}}
	if got := missingVocab(check, &duplicates); len(got) != 1 {
		t.Fatalf("duplicate-name missingVocab = %v", got)
	}
}
