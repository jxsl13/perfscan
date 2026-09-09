package config

import "testing"

func TestResidentCodebookContractCompileAndValidation(t *testing.T) {
	t.Parallel()
	contract := ResidentCodebookContract{Name: "iq2-s", ConfiguredSite: "example.com/backend.dispatch", ProducerCallable: "example.com/backend.exactGrid", UploadCallable: "example.com/backend.Device.Upload", DispatchCallable: "example.com/backend.Device.Dispatch", UploadDataArgument: 1, DispatchBufferArgument: 1, WideElementBytes: 4, PackedBitsPerSymbol: 2, SymbolCount: 8192, DistinctValueCount: 3, RowCount: 1024, SymbolsPerPackedWord: 8, PackedWordBits: 16, MaxCodebookBytes: 2048, ConfiguredEvidence: "hash-pinned", NativeBoundaryEvidence: "metal-buffer-4", ProducerReturnsFreshWideCodebook: true, ProducerExactAndInvariant: true, UploadCopiesSynchronously: true, UploadDoesNotRetainHostSlice: true, DeviceBufferImmutableDuringDispatch: true, HostPackingMatchesNativeDecode: true, NativeDecodeExactAndCheap: true, PairedResidentAndHostBenchmarkRequired: true, NumericalParityRequired: true, LifetimeAndSynchronizationValidationRequired: true}
	if !contract.Valid() {
		t.Fatal("complete resident-codebook contract is invalid")
	}
	tooSmall := contract
	tooSmall.MaxCodebookBytes = 2047
	if tooSmall.Valid() {
		t.Fatal("contract accepted a packed-word footprint above its byte ceiling")
	}
	padded := contract
	padded.SymbolCount, padded.RowCount, padded.SymbolsPerPackedWord, padded.PackedWordBits, padded.MaxCodebookBytes = 6, 2, 3, 16, 2
	if padded.Valid() {
		t.Fatal("contract counted dense symbol bits instead of padded packed words")
	}
	configuration := Config{ResidentCodebookContracts: []ResidentCodebookContract{contract}}
	compiled := configuration.Compile()
	configuration.ResidentCodebookContracts[0].Name = "mutated"
	if len(compiled.ResidentCodebookContracts) != 1 || compiled.ResidentCodebookContracts[0].Name != "iq2-s" {
		t.Fatalf("Compile did not clone contract: %+v", compiled.ResidentCodebookContracts)
	}
}
