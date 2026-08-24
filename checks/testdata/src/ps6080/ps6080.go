package ps6080

import (
	. "context"
	"errors"
	"fmt"
	"io"
	"os"
	"unsafe"
)

type QuantType uint8

const (
	Q4     QuantType = iota
	IQ2XXS           // want `quant variant IQ2XXS \(QuantType\) has storage coverage in quantBlockByteSize and portable decode coverage in portableDequantize but is absent from 3 of 3 reachable CPU matmul dispatch layers.*other backend matmul evidence mentions it in cudaQMatMul.*can still reach an unsupported-type fallback on CPU`
	IQ3
	ArchiveOnly //perfscan:quant-matmul-read-only retained for model inspection only.
)

type OtherType uint8

const (
	OtherA OtherType = iota
	OtherB
)

var errUnsupported = errors.New("unsupported quant type")

func quantBlockByteSize(quant QuantType) int {
	switch quant {
	case Q4, IQ2XXS, IQ3, ArchiveOnly:
		return 32
	default:
		return 0
	}
}

func portableDequantize(quant QuantType, source []byte) []float32 {
	switch quant {
	case Q4, IQ2XXS, IQ3, ArchiveOnly:
		return make([]float32, len(source))
	default:
		return nil
	}
}

func qMatMulFastSupported(quant QuantType) bool {
	return quant == Q4 || quant == IQ3
}

func qMatMulRow(quant QuantType) error {
	switch quant {
	case Q4, IQ3:
		return nil
	default:
		return errUnsupported
	}
}

func qMatMulGeneral(quant QuantType) error {
	decoders := map[QuantType]func(){
		Q4:  func() {},
		IQ3: func() {},
	}
	if decode := decoders[quant]; decode != nil {
		decode()
		return nil
	}
	return errUnsupported
}

func QMatMul(quant QuantType, rows int) error {
	if rows == 1 && qMatMulFastSupported(quant) {
		return qMatMulRow(quant)
	}
	return qMatMulGeneral(quant)
}

func cudaQuantSupported(quant QuantType) bool {
	switch quant {
	case Q4, IQ2XXS, IQ3:
		return true
	default:
		return false
	}
}

func cudaQMatMul(quant QuantType) error {
	if cudaQuantSupported(quant) {
		return nil
	}
	return errUnsupported
}

// Complete layered coverage stays silent.
type CompleteQuant uint8

const (
	CompleteA CompleteQuant = iota
	CompleteB
)

func completeByteSize(quant CompleteQuant) int {
	switch quant {
	case CompleteA, CompleteB:
		return 8
	default:
		return 0
	}
}

func completeDecode(quant CompleteQuant) []float32 {
	switch quant {
	case CompleteA, CompleteB:
		return []float32{}
	default:
		return nil
	}
}

func completeQMatMulSupported(quant CompleteQuant) bool {
	return quant == CompleteA || quant == CompleteB
}

func completeQMatMulDispatch(quant CompleteQuant) error {
	switch quant {
	case CompleteA, CompleteB:
		return nil
	default:
		return errUnsupported
	}
}

func CompleteQMatMul(quant CompleteQuant) error {
	if completeQMatMulSupported(quant) {
		return completeQMatMulDispatch(quant)
	}
	return errUnsupported
}

// A single CPU layer is not enough structural evidence.
type SingleLayerQuant uint8

const (
	SingleA SingleLayerQuant = iota
	SingleB
)

func singleByteSize(quant SingleLayerQuant) int {
	switch quant {
	case SingleA, SingleB:
		return 8
	default:
		return 0
	}
}

func singleDecode(quant SingleLayerQuant) []float32 {
	switch quant {
	case SingleA, SingleB:
		return []float32{}
	default:
		return nil
	}
}

func SingleQMatMul(quant SingleLayerQuant) error {
	switch quant {
	case SingleA:
		return nil
	default:
		return errUnsupported
	}
}

// Different enum types never contribute coverage to QuantType.
func unrelatedQMatMul(quant OtherType) bool {
	return quant == OtherA || quant == OtherB
}

// Role evidence propagates through direct helpers whose names are otherwise
// neutral. WrappedB is intentionally absent from both CPU layers.
type WrappedQuant uint8

const (
	WrappedA WrappedQuant = iota
	WrappedB              // want `quant variant WrappedB \(WrappedQuant\) has storage coverage in wrappedStorageCases and portable decode coverage in wrappedDecodeCases but is absent from 2 of 2 reachable CPU matmul dispatch layers`
	WrappedC
)

func wrappedStorageCases(quant WrappedQuant) int {
	switch quant {
	case WrappedA, WrappedB, WrappedC:
		return 8
	default:
		return 0
	}
}

func wrappedByteSize(quant WrappedQuant) int { return wrappedStorageCases(quant) }

func wrappedDecodeCases(quant WrappedQuant) []float32 {
	switch quant {
	case WrappedA, WrappedB, WrappedC:
		return []float32{}
	default:
		return nil
	}
}

func wrappedDecode(quant WrappedQuant) []float32 { return wrappedDecodeCases(quant) }

func wrappedFastSupported(quant WrappedQuant) bool {
	return quant == WrappedA || quant == WrappedC
}

func wrappedDispatch(quant WrappedQuant) error {
	switch quant {
	case WrappedA, WrappedC:
		return nil
	default:
		return errUnsupported
	}
}

func WrappedQMatMul(quant WrappedQuant) error {
	if wrappedFastSupported(quant) {
		return wrappedDispatch(quant)
	}
	return errUnsupported
}

// Ordered range guards do not enumerate a finite dispatch set and stay silent.
type RangeQuant uint8

const (
	RangeA RangeQuant = iota
	RangeB
	RangeC
)

func rangeByteSize(quant RangeQuant) int {
	switch quant {
	case RangeA, RangeB, RangeC:
		return 8
	default:
		return 0
	}
}

func rangeDecode(quant RangeQuant) []float32 {
	switch quant {
	case RangeA, RangeB, RangeC:
		return []float32{}
	default:
		return nil
	}
}

func rangeSupported(quant RangeQuant) bool { return quant >= RangeA && quant <= RangeC }
func rangeDispatch(quant RangeQuant) bool  { return quant >= RangeA && quant <= RangeC }

func RangeQMatMul(quant RangeQuant) bool {
	return rangeSupported(quant) && rangeDispatch(quant)
}

// A directive elsewhere may mention a constant in prose, but only an attached
// constant or dispatch-function directive suppresses coverage diagnostics.
type StaleQuant uint8

const (
	StaleA StaleQuant = iota
	StaleB            // want `quant variant StaleB \(StaleQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	StaleC
)

func staleByteSize(quant StaleQuant) int {
	switch quant {
	case StaleA, StaleB, StaleC:
		return 8
	default:
		return 0
	}
}

func staleDecode(quant StaleQuant) []float32 {
	switch quant {
	case StaleA, StaleB, StaleC:
		return []float32{}
	default:
		return nil
	}
}

func staleSupported(quant StaleQuant) bool { return quant == StaleA || quant == StaleC }

func staleDispatch(quant StaleQuant) bool {
	switch quant {
	case StaleA, StaleC:
		return true
	default:
		return false
	}
}

func StaleQMatMul(quant StaleQuant) bool {
	return staleSupported(quant) && staleDispatch(quant)
}

func unrelatedPolicy() {
	//perfscan:quant-matmul-coverage-validated StaleB is mentioned for unrelated policy documentation.
}

// A non-callable enum-keyed metadata map is not a dispatch layer.
type MetadataQuant uint8

const (
	MetadataA MetadataQuant = iota
	MetadataB
	MetadataC
)

func metadataByteSize(quant MetadataQuant) int {
	switch quant {
	case MetadataA, MetadataB, MetadataC:
		return 8
	default:
		return 0
	}
}

func metadataDecode(quant MetadataQuant) []float32 {
	switch quant {
	case MetadataA, MetadataB, MetadataC:
		return []float32{}
	default:
		return nil
	}
}

func metadataSupported(quant MetadataQuant) bool {
	return quant == MetadataA || quant == MetadataB || quant == MetadataC
}

func metadataDispatch(quant MetadataQuant) bool {
	switch quant {
	case MetadataA, MetadataB, MetadataC:
		return true
	default:
		return false
	}
}

func MetadataQMatMul(quant MetadataQuant) bool {
	labels := map[MetadataQuant]string{MetadataA: "a", MetadataC: "c"}
	_ = labels[quant]
	return metadataSupported(quant) && metadataDispatch(quant)
}

// Integer aliases describe the same runtime variant and are canonicalized.
type AliasQuant uint8

const (
	AliasA AliasQuant = iota
	AliasB
	AliasC
	AliasBCompat = AliasB
)

func aliasByteSize(quant AliasQuant) int {
	switch quant {
	case AliasA, AliasBCompat, AliasC:
		return 8
	default:
		return 0
	}
}

func aliasDecode(quant AliasQuant) []float32 {
	switch quant {
	case AliasA, AliasB, AliasC:
		return []float32{}
	default:
		return nil
	}
}

func aliasSupported(quant AliasQuant) bool {
	return quant == AliasA || quant == AliasBCompat || quant == AliasC
}

func aliasDispatch(quant AliasQuant) bool {
	switch quant {
	case AliasA, AliasB, AliasC:
		return true
	default:
		return false
	}
}

func AliasQMatMul(quant AliasQuant) bool {
	return aliasSupported(quant) && aliasDispatch(quant)
}

// A neutral helper shared by storage and decode is not independent evidence
// that every value it recognizes is both storable and decodable.
type SharedEvidenceQuant uint8

const (
	SharedEvidenceA SharedEvidenceQuant = iota
	SharedEvidenceB
	SharedEvidenceFuture
)

func sharedEvidenceKnown(quant SharedEvidenceQuant) bool {
	return quant == SharedEvidenceA || quant == SharedEvidenceB || quant == SharedEvidenceFuture
}

func sharedEvidenceByteSize(quant SharedEvidenceQuant) int {
	if !sharedEvidenceKnown(quant) {
		return 0
	}
	switch quant {
	case SharedEvidenceA, SharedEvidenceB:
		return 8
	default:
		return 0
	}
}

func sharedEvidenceDecode(quant SharedEvidenceQuant) []float32 {
	if !sharedEvidenceKnown(quant) {
		return nil
	}
	switch quant {
	case SharedEvidenceA, SharedEvidenceB:
		return []float32{}
	default:
		return nil
	}
}

func sharedEvidenceSupported(quant SharedEvidenceQuant) bool {
	return quant == SharedEvidenceA || quant == SharedEvidenceB
}

func sharedEvidenceDispatch(quant SharedEvidenceQuant) bool {
	switch quant {
	case SharedEvidenceA, SharedEvidenceB:
		return true
	default:
		return false
	}
}

func SharedEvidenceQMatMul(quant SharedEvidenceQuant) bool {
	return sharedEvidenceSupported(quant) && sharedEvidenceDispatch(quant)
}

// A suppression attached to an integer alias applies to the same runtime value.
type SuppressedAliasQuant uint8

const (
	SuppressedAliasA SuppressedAliasQuant = iota
	SuppressedAliasB
	SuppressedAliasC
	SuppressedAliasBCompat = SuppressedAliasB //perfscan:quant-matmul-read-only compatibility-only spelling.
)

func suppressedAliasByteSize(quant SuppressedAliasQuant) int {
	switch quant {
	case SuppressedAliasA, SuppressedAliasB, SuppressedAliasC:
		return 8
	default:
		return 0
	}
}

func suppressedAliasDecode(quant SuppressedAliasQuant) []float32 {
	switch quant {
	case SuppressedAliasA, SuppressedAliasBCompat, SuppressedAliasC:
		return []float32{}
	default:
		return nil
	}
}

func suppressedAliasSupported(quant SuppressedAliasQuant) bool {
	return quant == SuppressedAliasA || quant == SuppressedAliasC
}

func suppressedAliasDispatch(quant SuppressedAliasQuant) bool {
	switch quant {
	case SuppressedAliasA, SuppressedAliasC:
		return true
	default:
		return false
	}
}

func SuppressedAliasQMatMul(quant SuppressedAliasQuant) bool {
	return suppressedAliasSupported(quant) && suppressedAliasDispatch(quant)
}

// A function-level directive suppresses only that dispatch layer.
type ValidatedQuant uint8

const (
	ValidatedA ValidatedQuant = iota
	ValidatedB
	ValidatedC
)

func validatedByteSize(quant ValidatedQuant) int {
	switch quant {
	case ValidatedA, ValidatedB, ValidatedC:
		return 8
	default:
		return 0
	}
}

func validatedDecode(quant ValidatedQuant) []float32 {
	switch quant {
	case ValidatedA, ValidatedB, ValidatedC:
		return []float32{}
	default:
		return nil
	}
}

//perfscan:quant-matmul-coverage-validated ValidatedB intentionally uses the general route.
func validatedFastSupported(quant ValidatedQuant) bool {
	return quant == ValidatedA || quant == ValidatedC
}

func validatedGeneralDispatch(quant ValidatedQuant) bool {
	switch quant {
	case ValidatedA, ValidatedB, ValidatedC:
		return true
	default:
		return false
	}
}

func ValidatedQMatMul(quant ValidatedQuant) bool {
	return validatedFastSupported(quant) || validatedGeneralDispatch(quant)
}

// A validated layer still counts toward the independent dispatch threshold,
// but only omissions in the unvalidated layer are reportable.
type ValidatedThresholdQuant uint8

const (
	ValidatedThresholdA ValidatedThresholdQuant = iota
	ValidatedThresholdB
	ValidatedThresholdC // want `quant variant ValidatedThresholdC \(ValidatedThresholdQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers.*validatedThresholdRegular.*layers already mentioning it: validatedThresholdSpecialized.*incomplete fast/general route coverage`
)

func validatedThresholdByteSize(quant ValidatedThresholdQuant) int {
	switch quant {
	case ValidatedThresholdA, ValidatedThresholdB, ValidatedThresholdC:
		return 8
	default:
		return 0
	}
}

func validatedThresholdDecode(quant ValidatedThresholdQuant) []float32 {
	switch quant {
	case ValidatedThresholdA, ValidatedThresholdB, ValidatedThresholdC:
		return []float32{}
	default:
		return nil
	}
}

//perfscan:quant-matmul-coverage-validated ValidatedThresholdB intentionally uses the regular layer.
func validatedThresholdSpecialized(quant ValidatedThresholdQuant) bool {
	return quant == ValidatedThresholdA || quant == ValidatedThresholdC
}

func validatedThresholdRegular(quant ValidatedThresholdQuant) bool {
	return quant == ValidatedThresholdA || quant == ValidatedThresholdB
}

func ValidatedThresholdQMatMul(quant ValidatedThresholdQuant) bool {
	return validatedThresholdSpecialized(quant) || validatedThresholdRegular(quant)
}

// A global table reached only through validated functions suppresses its own
// omission after the references coalesce into one dispatch layer.
type AllValidatedMapQuant uint8

const (
	AllValidatedMapA AllValidatedMapQuant = iota
	AllValidatedMapB
	AllValidatedMapC
)

func allValidatedMapByteSize(quant AllValidatedMapQuant) int {
	switch quant {
	case AllValidatedMapA, AllValidatedMapB, AllValidatedMapC:
		return 8
	default:
		return 0
	}
}

func allValidatedMapDecode(quant AllValidatedMapQuant) []float32 {
	switch quant {
	case AllValidatedMapA, AllValidatedMapB, AllValidatedMapC:
		return []float32{}
	default:
		return nil
	}
}

var allValidatedMapDispatch = map[AllValidatedMapQuant]func(){
	AllValidatedMapA: func() {},
	AllValidatedMapB: func() {},
}

//perfscan:quant-matmul-coverage-validated AllValidatedMapC intentionally uses the regular layer.
func allValidatedMapFirst(quant AllValidatedMapQuant) bool {
	implementation := allValidatedMapDispatch[quant]
	return implementation != nil
}

//perfscan:quant-matmul-coverage-validated AllValidatedMapC intentionally uses the regular layer.
func allValidatedMapSecond(quant AllValidatedMapQuant) bool {
	implementation := allValidatedMapDispatch[quant]
	return implementation != nil
}

func allValidatedMapRegular(quant AllValidatedMapQuant) bool {
	return quant == AllValidatedMapA || quant == AllValidatedMapB ||
		quant == AllValidatedMapC
}

func AllValidatedMapQMatMul(quant AllValidatedMapQuant) bool {
	return allValidatedMapFirst(quant) || allValidatedMapSecond(quant) ||
		allValidatedMapRegular(quant)
}

// A shared global table loses function-level suppression when any coalesced
// reference comes from an unvalidated function.
type MixedValidatedMapQuant uint8

const (
	MixedValidatedMapA MixedValidatedMapQuant = iota
	MixedValidatedMapB
	MixedValidatedMapC // want `quant variant MixedValidatedMapC \(MixedValidatedMapQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers.*global map table.*layers already mentioning it: mixedValidatedMapRegular`
)

func mixedValidatedMapByteSize(quant MixedValidatedMapQuant) int {
	switch quant {
	case MixedValidatedMapA, MixedValidatedMapB, MixedValidatedMapC:
		return 8
	default:
		return 0
	}
}

func mixedValidatedMapDecode(quant MixedValidatedMapQuant) []float32 {
	switch quant {
	case MixedValidatedMapA, MixedValidatedMapB, MixedValidatedMapC:
		return []float32{}
	default:
		return nil
	}
}

var mixedValidatedMapDispatch = map[MixedValidatedMapQuant]func(){
	MixedValidatedMapA: func() {},
	MixedValidatedMapB: func() {},
}

//perfscan:quant-matmul-coverage-validated MixedValidatedMapC intentionally uses the regular layer.
func mixedValidatedMapSuppressed(quant MixedValidatedMapQuant) bool {
	implementation := mixedValidatedMapDispatch[quant]
	return implementation != nil
}

func mixedValidatedMapUnsuppressed(quant MixedValidatedMapQuant) bool {
	implementation := mixedValidatedMapDispatch[quant]
	return implementation != nil
}

func mixedValidatedMapRegular(quant MixedValidatedMapQuant) bool {
	return quant == MixedValidatedMapA || quant == MixedValidatedMapB ||
		quant == MixedValidatedMapC
}

func MixedValidatedMapQMatMul(quant MixedValidatedMapQuant) bool {
	return mixedValidatedMapSuppressed(quant) || mixedValidatedMapUnsuppressed(quant) ||
		mixedValidatedMapRegular(quant)
}

// Package-level callable maps participate when a CPU layer references them.
type GlobalTableQuant uint8

const (
	GlobalTableA GlobalTableQuant = iota
	GlobalTableB                  // want `quant variant GlobalTableB \(GlobalTableQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers.*globalDispatchLayer global map table`
	GlobalTableC
)

func globalTableByteSize(quant GlobalTableQuant) int {
	switch quant {
	case GlobalTableA, GlobalTableB, GlobalTableC:
		return 8
	default:
		return 0
	}
}

func globalTableDecode(quant GlobalTableQuant) []float32 {
	switch quant {
	case GlobalTableA, GlobalTableB, GlobalTableC:
		return []float32{}
	default:
		return nil
	}
}

var globalTableDispatch = map[GlobalTableQuant]func(){
	GlobalTableA: func() {},
	GlobalTableC: func() {},
}

func globalTableSupported(quant GlobalTableQuant) bool {
	return quant == GlobalTableA || quant == GlobalTableC
}

func globalDispatchLayer(quant GlobalTableQuant) bool {
	implementation := globalTableDispatch[quant]
	if implementation == nil {
		return false
	}
	implementation()
	return true
}

func GlobalTableQMatMul(quant GlobalTableQuant) bool {
	return globalTableSupported(quant) && globalDispatchLayer(quant)
}

// Non-callable maps remain valid storage/decode evidence even though the same
// shape is rejected as CPU dispatch evidence.
type StorageMapQuant uint8

const (
	StorageMapA StorageMapQuant = iota
	StorageMapB                 // want `quant variant StorageMapB \(StorageMapQuant\) has storage coverage in storageMapByteSize and portable decode coverage in storageMapDecode.*absent from 2 of 2 reachable CPU matmul dispatch layers`
	StorageMapC
)

var storageMapSizes = map[StorageMapQuant]int{
	StorageMapA: 8,
	StorageMapB: 16,
	StorageMapC: 24,
}

func storageMapByteSize(quant StorageMapQuant) int { return storageMapSizes[quant] }

func storageMapDecode(quant StorageMapQuant) []float32 {
	switch quant {
	case StorageMapA, StorageMapB, StorageMapC:
		return []float32{}
	default:
		return nil
	}
}

func storageMapSupported(quant StorageMapQuant) bool {
	return quant == StorageMapA || quant == StorageMapC
}

func storageMapDispatch(quant StorageMapQuant) bool {
	switch quant {
	case StorageMapA, StorageMapC:
		return true
	default:
		return false
	}
}

func StorageMapQMatMul(quant StorageMapQuant) bool {
	return storageMapSupported(quant) && storageMapDispatch(quant)
}

// Quant-specific size helpers are storage roots even when their names do not
// mention bytes, blocks, rows, layout, or stride.
type QuantSizeRoot uint8

const (
	SizeRootA QuantSizeRoot = iota
	SizeRootB               // want `quant variant SizeRootB \(QuantSizeRoot\) has storage coverage in quantSize and portable decode coverage in sizeRootDecode.*absent from 2 of 2 reachable CPU matmul dispatch layers`
	SizeRootC
)

func quantSize(quant QuantSizeRoot) int {
	switch quant {
	case SizeRootA, SizeRootB, SizeRootC:
		return 8
	default:
		return 0
	}
}

func sizeRootDecode(quant QuantSizeRoot) []float32 {
	switch quant {
	case SizeRootA, SizeRootB, SizeRootC:
		return []float32{}
	default:
		return nil
	}
}

func sizeRootSupported(quant QuantSizeRoot) bool {
	return quant == SizeRootA || quant == SizeRootC
}

func sizeRootDispatch(quant QuantSizeRoot) bool {
	switch quant {
	case SizeRootA, SizeRootC:
		return true
	default:
		return false
	}
}

func SizeRootQMatMul(quant QuantSizeRoot) bool {
	return sizeRootSupported(quant) && sizeRootDispatch(quant)
}

// Storage spellings must align with identifier boundaries. The suffix of
// "prototype" must not turn prototypeSize into a type-size storage contract.
type PrototypeSizeQuant uint8

const (
	PrototypeSizeA PrototypeSizeQuant = iota
	PrototypeSizeB
	PrototypeSizeC
)

func prototypeSize(quant PrototypeSizeQuant) int {
	switch quant {
	case PrototypeSizeA, PrototypeSizeB, PrototypeSizeC:
		return 8
	default:
		return 0
	}
}

func prototypeFormatDecode(quant PrototypeSizeQuant) []float32 {
	switch quant {
	case PrototypeSizeA, PrototypeSizeB, PrototypeSizeC:
		return []float32{}
	default:
		return nil
	}
}

func prototypeSupportLayer(quant PrototypeSizeQuant) bool {
	return quant == PrototypeSizeA || quant == PrototypeSizeC
}

func prototypeDispatchLayer(quant PrototypeSizeQuant) bool {
	switch quant {
	case PrototypeSizeA, PrototypeSizeC:
		return true
	default:
		return false
	}
}

func PrototypeContractQMatMul(quant PrototypeSizeQuant) bool {
	return prototypeSupportLayer(quant) && prototypeDispatchLayer(quant)
}

// The corresponding real type-size word sequence remains storage evidence.
type BoundaryStorageQuant uint8

const (
	BoundaryStorageA BoundaryStorageQuant = iota
	BoundaryStorageB                      // want `quant variant BoundaryStorageB \(BoundaryStorageQuant\) has storage coverage in quantTypeSize.*absent from 2 of 2 reachable CPU matmul dispatch layers`
	BoundaryStorageC
)

func quantTypeSize(quant BoundaryStorageQuant) int {
	switch quant {
	case BoundaryStorageA, BoundaryStorageB, BoundaryStorageC:
		return 8
	default:
		return 0
	}
}

func boundaryStorageDecode(quant BoundaryStorageQuant) []float32 {
	switch quant {
	case BoundaryStorageA, BoundaryStorageB, BoundaryStorageC:
		return []float32{}
	default:
		return nil
	}
}

func boundaryStorageSupport(quant BoundaryStorageQuant) bool {
	return quant == BoundaryStorageA || quant == BoundaryStorageC
}

func boundaryStorageDispatch(quant BoundaryStorageQuant) bool {
	switch quant {
	case BoundaryStorageA, BoundaryStorageC:
		return true
	default:
		return false
	}
}

func BoundaryQMatMul(quant BoundaryStorageQuant) bool {
	return boundaryStorageSupport(quant) && boundaryStorageDispatch(quant)
}

// Package dispatch maps built with make and populated by guaranteed direct
// init assignments are equivalent to static map literals.
type InitMadeTableQuant uint8

const (
	InitMadeTableA InitMadeTableQuant = iota
	InitMadeTableB                    // want `quant variant InitMadeTableB \(InitMadeTableQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	InitMadeTableC
)

func initMadeTableByteSize(quant InitMadeTableQuant) int {
	switch quant {
	case InitMadeTableA, InitMadeTableB, InitMadeTableC:
		return 8
	default:
		return 0
	}
}

func initMadeTableDecode(quant InitMadeTableQuant) []float32 {
	switch quant {
	case InitMadeTableA, InitMadeTableB, InitMadeTableC:
		return []float32{}
	default:
		return nil
	}
}

type initMadeDispatchMap map[InitMadeTableQuant]func()

var initMadeTableDispatch = make(initMadeDispatchMap, 3)

const initMadeTableDisabled = false

func init() {
	if initMadeTableDisabled {
		return
	}
	initMadeTableDispatch[InitMadeTableA] = func() {}
	initMadeTableDispatch[InitMadeTableC] = func() {}
}

func initMadeTableSupported(quant InitMadeTableQuant) bool {
	return quant == InitMadeTableA || quant == InitMadeTableC
}

func InitMadeTableQMatMul(quant InitMadeTableQuant) bool {
	implementation := initMadeTableDispatch[quant]
	if implementation == nil {
		return false
	}
	implementation()
	return initMadeTableSupported(quant)
}

// An unconditional nested block preserves the init function's execution
// guarantee and is not a conditional/helper-populated table.
type BareBlockInitQuant uint8

const (
	BareBlockInitA BareBlockInitQuant = iota
	BareBlockInitB                    // want `quant variant BareBlockInitB \(BareBlockInitQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	BareBlockInitC
)

func bareBlockInitByteSize(quant BareBlockInitQuant) int {
	switch quant {
	case BareBlockInitA, BareBlockInitB, BareBlockInitC:
		return 8
	default:
		return 0
	}
}

func bareBlockInitDecode(quant BareBlockInitQuant) []float32 {
	switch quant {
	case BareBlockInitA, BareBlockInitB, BareBlockInitC:
		return []float32{}
	default:
		return nil
	}
}

var bareBlockInitDispatch = make(map[BareBlockInitQuant]func())

func init() {
	{
		bareBlockInitDispatch[BareBlockInitA] = func() {}
		bareBlockInitDispatch[BareBlockInitC] = func() {}
	}
}

func bareBlockInitSupported(quant BareBlockInitQuant) bool {
	return quant == BareBlockInitA || quant == BareBlockInitC
}

func BareBlockInitQMatMul(quant BareBlockInitQuant) bool {
	implementation := bareBlockInitDispatch[quant]
	if implementation == nil {
		return false
	}
	implementation()
	return bareBlockInitSupported(quant)
}

// Any later mutation still invalidates an init-populated table.
type MutatedInitTableQuant uint8

const (
	MutatedInitTableA MutatedInitTableQuant = iota
	MutatedInitTableB
	MutatedInitTableC
)

func mutatedInitTableByteSize(quant MutatedInitTableQuant) int {
	switch quant {
	case MutatedInitTableA, MutatedInitTableB, MutatedInitTableC:
		return 8
	default:
		return 0
	}
}

func mutatedInitTableDecode(quant MutatedInitTableQuant) []float32 {
	switch quant {
	case MutatedInitTableA, MutatedInitTableB, MutatedInitTableC:
		return []float32{}
	default:
		return nil
	}
}

var mutatedInitTableDispatch = make(map[MutatedInitTableQuant]func())

func init() {
	mutatedInitTableDispatch[MutatedInitTableA] = func() {}
	mutatedInitTableDispatch[MutatedInitTableC] = func() {}
}

func mutateInitTableLater() {
	mutatedInitTableDispatch[MutatedInitTableB] = func() {}
}

func mutatedInitTableSupported(quant MutatedInitTableQuant) bool {
	return quant == MutatedInitTableA || quant == MutatedInitTableC
}

func MutatedInitTableQMatMul(quant MutatedInitTableQuant) bool {
	_, ok := mutatedInitTableDispatch[quant]
	return ok && mutatedInitTableSupported(quant)
}

// Complete init construction remains silent, while a static nil entry is
// exclusion evidence rather than support.
type CompleteInitTableQuant uint8

const (
	CompleteInitTableA CompleteInitTableQuant = iota
	CompleteInitTableB
	CompleteInitTableC
)

func completeInitTableByteSize(quant CompleteInitTableQuant) int {
	switch quant {
	case CompleteInitTableA, CompleteInitTableB, CompleteInitTableC:
		return 8
	default:
		return 0
	}
}

func completeInitTableDecode(quant CompleteInitTableQuant) []float32 {
	switch quant {
	case CompleteInitTableA, CompleteInitTableB, CompleteInitTableC:
		return []float32{}
	default:
		return nil
	}
}

var completeInitTableDispatch = make(map[CompleteInitTableQuant]func())

func init() {
	completeInitTableDispatch[CompleteInitTableA] = func() {}
	completeInitTableDispatch[CompleteInitTableB] = func() {}
	completeInitTableDispatch[CompleteInitTableC] = func() {}
}

func completeInitTableSupported(quant CompleteInitTableQuant) bool {
	return quant == CompleteInitTableA || quant == CompleteInitTableB || quant == CompleteInitTableC
}

func CompleteInitTableQMatMul(quant CompleteInitTableQuant) bool {
	_, ok := completeInitTableDispatch[quant]
	return ok && completeInitTableSupported(quant)
}

type NilInitTableQuant uint8

const (
	NilInitTableA NilInitTableQuant = iota
	NilInitTableB                   // want `quant variant NilInitTableB \(NilInitTableQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	NilInitTableC
)

func nilInitTableByteSize(quant NilInitTableQuant) int {
	switch quant {
	case NilInitTableA, NilInitTableB, NilInitTableC:
		return 8
	default:
		return 0
	}
}

func nilInitTableDecode(quant NilInitTableQuant) []float32 {
	switch quant {
	case NilInitTableA, NilInitTableB, NilInitTableC:
		return []float32{}
	default:
		return nil
	}
}

var nilInitTableDispatch = make(map[NilInitTableQuant]func())

func init() {
	nilInitTableDispatch[NilInitTableA] = func() {}
	nilInitTableDispatch[NilInitTableB] = nil
	nilInitTableDispatch[NilInitTableC] = func() {}
}

func nilInitTableSupported(quant NilInitTableQuant) bool {
	return quant == NilInitTableA || quant == NilInitTableB || quant == NilInitTableC
}

func NilInitTableQMatMul(quant NilInitTableQuant) bool {
	implementation := nilInitTableDispatch[quant]
	return implementation != nil && nilInitTableSupported(quant)
}

// Non-straight-line, ambiguous, aliased, helper-populated, and externally
// mutable init tables remain conservative rather than manufacturing coverage.
type RejectedInitTableQuant uint8

const (
	RejectedInitTableA RejectedInitTableQuant = iota
	RejectedInitTableB                        // want `quant variant RejectedInitTableB \(RejectedInitTableQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	RejectedInitTableC
)

func rejectedInitTableByteSize(quant RejectedInitTableQuant) int {
	switch quant {
	case RejectedInitTableA, RejectedInitTableB, RejectedInitTableC:
		return 8
	default:
		return 0
	}
}

func rejectedInitTableDecode(quant RejectedInitTableQuant) []float32 {
	switch quant {
	case RejectedInitTableA, RejectedInitTableB, RejectedInitTableC:
		return []float32{}
	default:
		return nil
	}
}

var rejectedConditionalInitTable = make(map[RejectedInitTableQuant]func())
var rejectedDuplicateInitTable = make(map[RejectedInitTableQuant]func())
var rejectedDynamicInitTable = make(map[RejectedInitTableQuant]func())
var rejectedAliasInitTable = make(map[RejectedInitTableQuant]func())
var rejectedHelperInitTable = make(map[RejectedInitTableQuant]func())
var rejectedExportedAliasInitTable = make(map[RejectedInitTableQuant]func())
var RejectedExportedAliasInitTable = rejectedExportedAliasInitTable
var rejectedConstantExitInitTable = make(map[RejectedInitTableQuant]func())
var recoveredNamedInitTable = make(map[RejectedInitTableQuant]func())
var recoveredIIFEInitTable = make(map[RejectedInitTableQuant]func())
var recoveredReturningInitTable = make(map[RejectedInitTableQuant]func())
var rejectedInitEnabled bool
var rejectedInitKey = RejectedInitTableA

const rejectedInitExits = true

func populateRejectedHelperInitTable() {
	rejectedHelperInitTable[RejectedInitTableA] = func() {}
	rejectedHelperInitTable[RejectedInitTableC] = func() {}
}

func init() {
	if rejectedInitEnabled {
		rejectedConditionalInitTable[RejectedInitTableA] = func() {}
		rejectedConditionalInitTable[RejectedInitTableC] = func() {}
	}
	rejectedDuplicateInitTable[RejectedInitTableA] = func() {}
	rejectedDuplicateInitTable[RejectedInitTableA] = func() {}
	rejectedDynamicInitTable[rejectedInitKey] = func() {}
	alias := rejectedAliasInitTable
	alias[RejectedInitTableA] = func() {}
	alias[RejectedInitTableC] = func() {}
	populateRejectedHelperInitTable()
	rejectedExportedAliasInitTable[RejectedInitTableA] = func() {}
	rejectedExportedAliasInitTable[RejectedInitTableC] = func() {}
}

func init() {
	if rejectedInitExits {
		return
	}
	rejectedConstantExitInitTable[RejectedInitTableA] = func() {}
	rejectedConstantExitInitTable[RejectedInitTableC] = func() {}
}

func recoveredInitAlwaysPanics() {
	panic("stop")
}

func recoveredInitReturns() {}

func init() {
	defer func() { _ = recover() }()
	recoveredInitAlwaysPanics()
	recoveredNamedInitTable[RejectedInitTableA] = func() {}
	recoveredNamedInitTable[RejectedInitTableC] = func() {}
}

func init() {
	defer func() { _ = recover() }()
	func() { panic("stop") }()
	recoveredIIFEInitTable[RejectedInitTableA] = func() {}
	recoveredIIFEInitTable[RejectedInitTableC] = func() {}
}

func init() {
	defer func() { _ = recover() }()
	recoveredInitReturns()
	recoveredReturningInitTable[RejectedInitTableA] = func() {}
	recoveredReturningInitTable[RejectedInitTableC] = func() {}
}

func rejectedInitTableSupported(quant RejectedInitTableQuant) bool {
	return quant == RejectedInitTableA || quant == RejectedInitTableC
}

func RejectedConditionalInitQMatMul(quant RejectedInitTableQuant) bool {
	_, ok := rejectedConditionalInitTable[quant]
	return ok && rejectedInitTableSupported(quant)
}

func RejectedDuplicateInitQMatMul(quant RejectedInitTableQuant) bool {
	_, ok := rejectedDuplicateInitTable[quant]
	return ok && rejectedInitTableSupported(quant)
}

func RejectedDynamicInitQMatMul(quant RejectedInitTableQuant) bool {
	_, ok := rejectedDynamicInitTable[quant]
	return ok && rejectedInitTableSupported(quant)
}

func RejectedAliasInitQMatMul(quant RejectedInitTableQuant) bool {
	_, ok := rejectedAliasInitTable[quant]
	return ok && rejectedInitTableSupported(quant)
}

func RejectedHelperInitQMatMul(quant RejectedInitTableQuant) bool {
	_, ok := rejectedHelperInitTable[quant]
	return ok && rejectedInitTableSupported(quant)
}

func RejectedExportedAliasInitQMatMul(quant RejectedInitTableQuant) bool {
	_, ok := RejectedExportedAliasInitTable[quant]
	return ok && rejectedInitTableSupported(quant)
}

func RejectedConstantExitInitQMatMul(quant RejectedInitTableQuant) bool {
	_, ok := rejectedConstantExitInitTable[quant]
	return ok && rejectedInitTableSupported(quant)
}

func RecoveredNamedInitQMatMul(quant RejectedInitTableQuant) bool {
	_, ok := recoveredNamedInitTable[quant]
	return ok && rejectedInitTableSupported(quant)
}

func RecoveredIIFEInitQMatMul(quant RejectedInitTableQuant) bool {
	_, ok := recoveredIIFEInitTable[quant]
	return ok && rejectedInitTableSupported(quant)
}

func RecoveredReturningInitQMatMul(quant RejectedInitTableQuant) bool {
	_, ok := recoveredReturningInitTable[quant]
	return ok && rejectedInitTableSupported(quant)
}

// Layered evidence makes even single-case guards meaningful.
type SingleCaseQuant uint8

const (
	SingleCaseA SingleCaseQuant = iota
	SingleCaseB                 // want `quant variant SingleCaseB \(SingleCaseQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	SingleCaseC                 // want `quant variant SingleCaseC \(SingleCaseQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
)

func singleCaseByteSize(quant SingleCaseQuant) int {
	switch quant {
	case SingleCaseA, SingleCaseB, SingleCaseC:
		return 8
	default:
		return 0
	}
}

func singleCaseDecode(quant SingleCaseQuant) []float32 {
	switch quant {
	case SingleCaseA, SingleCaseB, SingleCaseC:
		return []float32{}
	default:
		return nil
	}
}

func singleCaseSupported(quant SingleCaseQuant) bool { return quant == SingleCaseA }

func singleCaseDispatch(quant SingleCaseQuant) bool {
	switch quant {
	case SingleCaseA:
		return true
	default:
		return false
	}
}

func SingleCaseQMatMul(quant SingleCaseQuant) bool {
	return singleCaseSupported(quant) && singleCaseDispatch(quant)
}

// Direct enum-return helpers are not finite support or dispatch predicates.
type ReturnedQuant uint8

const (
	ReturnedA ReturnedQuant = iota
	ReturnedB
	ReturnedC
)

func returnedByteSize(quant ReturnedQuant) int {
	switch quant {
	case ReturnedA, ReturnedB, ReturnedC:
		return 8
	default:
		return 0
	}
}

func returnedDecode(quant ReturnedQuant) []float32 {
	switch quant {
	case ReturnedA, ReturnedB, ReturnedC:
		return []float32{}
	default:
		return nil
	}
}

func returnedDefaultOne() ReturnedQuant { return ReturnedA }
func returnedDefaultTwo() ReturnedQuant { return ReturnedA }

func ReturnedQMatMul() bool {
	return returnedDefaultOne() == returnedDefaultTwo()
}

// One package-level table remains one dispatch layer even when two reachable
// wrappers reference it.
type SharedGlobalQuant uint8

const (
	SharedGlobalA SharedGlobalQuant = iota
	SharedGlobalB
	SharedGlobalC
)

func sharedGlobalByteSize(quant SharedGlobalQuant) int {
	switch quant {
	case SharedGlobalA, SharedGlobalB, SharedGlobalC:
		return 8
	default:
		return 0
	}
}

func sharedGlobalDecode(quant SharedGlobalQuant) []float32 {
	switch quant {
	case SharedGlobalA, SharedGlobalB, SharedGlobalC:
		return []float32{}
	default:
		return nil
	}
}

var sharedGlobalDispatch = map[SharedGlobalQuant]func(){
	SharedGlobalA: func() {},
	SharedGlobalC: func() {},
}

func sharedGlobalLayerOne(quant SharedGlobalQuant) bool { return sharedGlobalDispatch[quant] != nil }
func sharedGlobalLayerTwo(quant SharedGlobalQuant) bool { return sharedGlobalDispatch[quant] != nil }

func SharedGlobalQMatMul(quant SharedGlobalQuant) bool {
	return sharedGlobalLayerOne(quant) && sharedGlobalLayerTwo(quant)
}

// Internal wire/storage IDs can be untyped while the public API exposes a
// named enum. Exact alias declarations bridge those two spellings without
// conflating unrelated enums that happen to reuse the same numeric values.
const (
	bridgedRawA = 101
	bridgedRawB = 102
	bridgedRawC = 103
)

type BridgedQuant uint32

const (
	BridgedA BridgedQuant = bridgedRawA
	BridgedB BridgedQuant = bridgedRawB // want `quant variant BridgedB \(BridgedQuant\) has storage coverage in bridgedByteSize and portable decode coverage in bridgedDecode but is absent from 2 of 2 reachable CPU matmul dispatch layers`
	BridgedC BridgedQuant = bridgedRawC
)

func bridgedByteSize(raw uint32) int {
	switch raw {
	case bridgedRawA, bridgedRawB, bridgedRawC:
		return 8
	default:
		return 0
	}
}

func bridgedDecode(quant BridgedQuant) []float32 {
	switch quant {
	case BridgedA, BridgedB, BridgedC:
		return []float32{}
	default:
		return nil
	}
}

func bridgedSupported(quant BridgedQuant) bool {
	return quant == BridgedA || quant == BridgedC
}

func bridgedDispatch(quant BridgedQuant) bool {
	switch quant {
	case BridgedA, BridgedC:
		return true
	default:
		return false
	}
}

// BridgedQMatMul is the portable CPU fallback when CUDA is unavailable. A
// backend mentioned in CPU documentation must not reclassify this root.
func BridgedQMatMul(quant BridgedQuant) bool {
	return bridgedSupported(quant) && bridgedDispatch(quant)
}

// Negative predicates describe an open support set. They exclude only the
// named value rather than turning every other enum value into a missing case.
type NegativeGuardQuant uint8

const (
	NegativeGuardA NegativeGuardQuant = iota
	NegativeGuardB                    // want `quant variant NegativeGuardB \(NegativeGuardQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	NegativeGuardC
)

func negativeGuardByteSize(q NegativeGuardQuant) int {
	switch q {
	case NegativeGuardA, NegativeGuardB, NegativeGuardC:
		return 8
	default:
		return 0
	}
}

func negativeGuardDecode(q NegativeGuardQuant) []float32 {
	switch q {
	case NegativeGuardA, NegativeGuardB, NegativeGuardC:
		return []float32{}
	default:
		return nil
	}
}

func negativeGuardSupported(q NegativeGuardQuant) bool { return q != NegativeGuardB }
func negativeGuardDispatch(q NegativeGuardQuant) bool {
	switch q {
	case NegativeGuardA, NegativeGuardC:
		return true
	default:
		return false
	}
}
func NegativeGuardQMatMul(q NegativeGuardQuant) bool {
	return negativeGuardSupported(q) && negativeGuardDispatch(q)
}

// Dormant tables and tables with an unrelated key domain cannot manufacture
// dispatch coverage merely by existing in a matmul function.
type DormantQuant uint8

const (
	DormantA DormantQuant = iota
	DormantB              // want `quant variant DormantB \(DormantQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	DormantC
)

func dormantByteSize(q DormantQuant) int {
	switch q {
	case DormantA, DormantB, DormantC:
		return 8
	default:
		return 0
	}
}
func dormantDecode(q DormantQuant) []float32 {
	switch q {
	case DormantA, DormantB, DormantC:
		return []float32{}
	default:
		return nil
	}
}
func dormantSupport(q DormantQuant) bool { return q == DormantA || q == DormantC }
func dormantDispatch(q DormantQuant) bool {
	switch q {
	case DormantA, DormantC:
		return true
	default:
		return false
	}
}
func DormantQMatMul(q DormantQuant) bool {
	_ = map[DormantQuant]func(){DormantA: func() {}, DormantB: func() {}, DormantC: func() {}}
	wrongKey := map[any]func(){DormantA: func() {}, DormantB: func() {}, DormantC: func() {}}
	_ = wrongKey[q]
	constantOnly := map[DormantQuant]func(){DormantA: func() {}, DormantB: func() {}, DormantC: func() {}}
	_ = constantOnly[DormantA]
	dead := map[DormantQuant]func(){DormantA: func() {}, DormantB: func() {}, DormantC: func() {}}
	if false {
		_ = dead[q]
	}
	mutated := map[DormantQuant]func(){DormantA: func() {}, DormantB: func() {}, DormantC: func() {}}
	delete(mutated, DormantB)
	_ = mutated[q]
	return dormantSupport(q) && dormantDispatch(q)
}

// Callable map entries are statically non-nil regardless of a handler's name.
type HandlerQuant uint8

const (
	HandlerA HandlerQuant = iota
	HandlerB
	HandlerC
)

func handlerByteSize(q HandlerQuant) int {
	switch q {
	case HandlerA, HandlerB, HandlerC:
		return 8
	default:
		return 0
	}
}
func handlerDecode(q HandlerQuant) []float32 {
	switch q {
	case HandlerA, HandlerB, HandlerC:
		return []float32{}
	default:
		return nil
	}
}
func errorHandler() {}
func HandlerQMatMul(q HandlerQuant) bool {
	one := map[HandlerQuant]func(){HandlerA: errorHandler, HandlerB: errorHandler, HandlerC: errorHandler}
	two := map[HandlerQuant]func(){HandlerA: errorHandler, HandlerB: errorHandler, HandlerC: errorHandler}
	return one[q] != nil && two[q] != nil
}

// Direct IIFEs are synchronous dispatch layers and generic enum conversions
// retain the compared constant's identity.
type GenericQuant uint8

const (
	GenericA GenericQuant = iota
	GenericB              // want `quant variant GenericB \(GenericQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	GenericC
)

func genericByteSize(q GenericQuant) int {
	switch q {
	case GenericA, GenericB, GenericC:
		return 8
	default:
		return 0
	}
}
func genericDecode(q GenericQuant) []float32 {
	switch q {
	case GenericA, GenericB, GenericC:
		return []float32{}
	default:
		return nil
	}
}
func genericSupports[T ~uint8](q T) bool { return q == T(GenericA) || q == T(GenericC) }
func GenericQMatMul(q GenericQuant) bool {
	return genericSupports(q) && func() bool {
		switch q {
		case GenericA, GenericC:
			return true
		default:
			return false
		}
	}()
}

// Dead short-circuit calls and calls after panic do not import fake complete
// coverage into the reachable CPU graph.
type ReachableQuant uint8

const (
	ReachableA ReachableQuant = iota
	ReachableB                // want `quant variant ReachableB \(ReachableQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	ReachableC
)

func reachableByteSize(q ReachableQuant) int {
	switch q {
	case ReachableA, ReachableB, ReachableC:
		return 8
	default:
		return 0
	}
}
func reachableDecode(q ReachableQuant) []float32 {
	switch q {
	case ReachableA, ReachableB, ReachableC:
		return []float32{}
	default:
		return nil
	}
}
func reachablePartialOne(q ReachableQuant) bool { return q == ReachableA || q == ReachableC }
func reachablePartialTwo(q ReachableQuant) bool { return q == ReachableA || q == ReachableC }
func reachableFakeFull(q ReachableQuant) bool {
	return q == ReachableA || q == ReachableB || q == ReachableC
}
func reachablePanicTail(q ReachableQuant) bool {
	panic("stop")
	return reachableFakeFull(q)
}
func ReachableQMatMul(q ReachableQuant) bool {
	_ = false && reachableFakeFull(q)
	dormant := func() bool { return reachableFakeFull(q) }
	if false {
		_ = reachablePanicTail(q)
		_ = dormant()
	}
	panic := func(any) {}
	panic(nil)
	return reachablePartialOne(q) && reachablePartialTwo(q)
}

// Separate non-terminating guards are distinct validation stages even when
// they share the same rows==1 context.
type StagedQuant uint8

const (
	StagedA StagedQuant = iota
	StagedB             // want `quant variant StagedB \(StagedQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	StagedC
)

func stagedByteSize(q StagedQuant) int {
	switch q {
	case StagedA, StagedB, StagedC:
		return 8
	default:
		return 0
	}
}
func stagedDecode(q StagedQuant) []float32 {
	switch q {
	case StagedA, StagedB, StagedC:
		return []float32{}
	default:
		return nil
	}
}
func StagedQMatMul(q StagedQuant, rows int) bool {
	accepted := false
	if rows == 1 && (q == StagedA || q == StagedB || q == StagedC) {
		accepted = true
	}
	if rows == 1 && (q == StagedA || q == StagedC) {
		accepted = accepted && true
	}
	return accepted
}

// Rejected, empty/fallthrough, and naked-result cases do not count as support.
type OutcomeQuant uint8

const (
	OutcomeA OutcomeQuant = iota
	OutcomeB              // want `quant variant OutcomeB \(OutcomeQuant\).*absent from 4 of 4 reachable CPU matmul dispatch layers`
	OutcomeC
)

func outcomeByteSize(q OutcomeQuant) int {
	switch q {
	case OutcomeA, OutcomeB, OutcomeC:
		return 8
	default:
		return 0
	}
}
func outcomeDecode(q OutcomeQuant) []float32 {
	switch q {
	case OutcomeA, OutcomeB, OutcomeC:
		return []float32{}
	default:
		return nil
	}
}
func outcomeEmpty(q OutcomeQuant) bool {
	switch q {
	case OutcomeA, OutcomeC:
		return true
	case OutcomeB:
	}
	return false
}
func outcomeFallthrough(q OutcomeQuant) bool {
	switch q {
	case OutcomeA, OutcomeC:
		return true
	case OutcomeB:
		fallthrough
	default:
		return false
	}
}
func outcomeNaked(q OutcomeQuant) (ok bool) {
	switch q {
	case OutcomeA, OutcomeC:
		return true
	case OutcomeB:
		ok = false
		return
	}
	return false
}
func outcomeSetupThenFallback(q OutcomeQuant) bool {
	switch q {
	case OutcomeA, OutcomeC:
		return true
	case OutcomeB:
		_ = q
	}
	return false
}
func OutcomeQMatMul(q OutcomeQuant) bool {
	return outcomeEmpty(q) && outcomeFallthrough(q) && outcomeNaked(q) && outcomeSetupThenFallback(q)
}

// Rejecting guards do not make their complements supported when the common
// continuation also rejects every value.
type RejectAllQuant uint8

const (
	RejectAllA RejectAllQuant = iota // want `quant variant RejectAllA \(RejectAllQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	RejectAllB                       // want `quant variant RejectAllB \(RejectAllQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	RejectAllC                       // want `quant variant RejectAllC \(RejectAllQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
)

func rejectAllByteSize(q RejectAllQuant) int {
	switch q {
	case RejectAllA, RejectAllB, RejectAllC:
		return 8
	default:
		return 0
	}
}
func rejectAllDecode(q RejectAllQuant) []float32 {
	switch q {
	case RejectAllA, RejectAllB, RejectAllC:
		return []float32{}
	default:
		return nil
	}
}
func rejectAllOne(q RejectAllQuant) bool {
	if q == RejectAllB {
		return false
	}
	return false
}
func rejectAllTwo(q RejectAllQuant) bool {
	if q == RejectAllB {
		return false
	}
	return false
}
func RejectAllQMatMul(q RejectAllQuant) bool { return rejectAllOne(q) || rejectAllTwo(q) }

// A setup-only matching guard inherits its rejecting continuation; the setup
// does not make the guarded quant variant supported.
type SetupRejectQuant uint8

const (
	SetupRejectA SetupRejectQuant = iota // want `quant variant SetupRejectA \(SetupRejectQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	SetupRejectB                         // want `quant variant SetupRejectB \(SetupRejectQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	SetupRejectC                         // want `quant variant SetupRejectC \(SetupRejectQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
)

func setupRejectByteSize(q SetupRejectQuant) int {
	switch q {
	case SetupRejectA, SetupRejectB, SetupRejectC:
		return 8
	default:
		return 0
	}
}

func setupRejectDecode(q SetupRejectQuant) []float32 {
	switch q {
	case SetupRejectA, SetupRejectB, SetupRejectC:
		return []float32{}
	default:
		return nil
	}
}

func setupRejectOne(q SetupRejectQuant) bool {
	if q == SetupRejectB {
		_ = q
	}
	return false
}

func setupRejectTwo(q SetupRejectQuant) bool {
	if q == SetupRejectB {
		_ = q
	}
	return false
}

func SetupRejectQMatMul(q SetupRejectQuant) bool {
	return setupRejectOne(q) || setupRejectTwo(q)
}

// Bare blocks do not hide a rejecting continuation from a setup-only guard.
type NestedSetupRejectQuant uint8

const (
	NestedSetupRejectA NestedSetupRejectQuant = iota // want `quant variant NestedSetupRejectA \(NestedSetupRejectQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	NestedSetupRejectB                               // want `quant variant NestedSetupRejectB \(NestedSetupRejectQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	NestedSetupRejectC                               // want `quant variant NestedSetupRejectC \(NestedSetupRejectQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
)

func nestedSetupRejectByteSize(q NestedSetupRejectQuant) int {
	switch q {
	case NestedSetupRejectA, NestedSetupRejectB, NestedSetupRejectC:
		return 8
	default:
		return 0
	}
}

func nestedSetupRejectDecode(q NestedSetupRejectQuant) []float32 {
	switch q {
	case NestedSetupRejectA, NestedSetupRejectB, NestedSetupRejectC:
		return []float32{}
	default:
		return nil
	}
}

func nestedSetupRejectOne(q NestedSetupRejectQuant) bool {
	{
		if q == NestedSetupRejectB {
			_ = q
		}
	}
	return false
}

func nestedSetupRejectTwo(q NestedSetupRejectQuant) bool {
	{
		{
			if q == NestedSetupRejectB {
				_ = q
			}
		}
	}
	return false
}

func NestedSetupRejectQMatMul(q NestedSetupRejectQuant) bool {
	return nestedSetupRejectOne(q) || nestedSetupRejectTwo(q)
}

// Reaching the end of a void function is a successful continuation, even
// after ordinary setup or a conditional rejecting path.
type VoidSetupQuant uint8

const (
	VoidSetupA VoidSetupQuant = iota // want `quant variant VoidSetupA \(VoidSetupQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	VoidSetupB
	VoidSetupC // want `quant variant VoidSetupC \(VoidSetupQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
)

var voidSetupShouldFail bool

func voidSetupByteSize(q VoidSetupQuant) int {
	switch q {
	case VoidSetupA, VoidSetupB, VoidSetupC:
		return 8
	default:
		return 0
	}
}

func voidSetupDecode(q VoidSetupQuant) []float32 {
	switch q {
	case VoidSetupA, VoidSetupB, VoidSetupC:
		return []float32{}
	default:
		return nil
	}
}

func voidSetupOne(q VoidSetupQuant) {
	if q == VoidSetupB {
		_ = q
	}
	_ = q
}

func voidSetupTwo(q VoidSetupQuant) {
	if q == VoidSetupB {
		_ = q
	}
	if voidSetupShouldFail {
		panic("failed")
	}
}

func VoidSetupQMatMul(q VoidSetupQuant) {
	voidSetupOne(q)
	voidSetupTwo(q)
}

// A repeated guard is correlated with the already-matched path; its rejecting
// branch cannot be diluted by the unreachable successful fallback.
type CorrelatedSetupRejectQuant uint8

const (
	CorrelatedSetupRejectA CorrelatedSetupRejectQuant = iota // want `quant variant CorrelatedSetupRejectA \(CorrelatedSetupRejectQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	CorrelatedSetupRejectB                                   // want `quant variant CorrelatedSetupRejectB \(CorrelatedSetupRejectQuant\).*absent from 4 of 4 reachable CPU matmul dispatch layers`
	CorrelatedSetupRejectC                                   // want `quant variant CorrelatedSetupRejectC \(CorrelatedSetupRejectQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
)

func correlatedSetupRejectByteSize(q CorrelatedSetupRejectQuant) int {
	switch q {
	case CorrelatedSetupRejectA, CorrelatedSetupRejectB, CorrelatedSetupRejectC:
		return 8
	default:
		return 0
	}
}

func correlatedSetupRejectDecode(q CorrelatedSetupRejectQuant) []float32 {
	switch q {
	case CorrelatedSetupRejectA, CorrelatedSetupRejectB, CorrelatedSetupRejectC:
		return []float32{}
	default:
		return nil
	}
}

func correlatedSetupRejectOne(q CorrelatedSetupRejectQuant) bool {
	if q == CorrelatedSetupRejectB {
		_ = q
	}
	if q == CorrelatedSetupRejectB {
		return false
	}
	return true
}

func correlatedSetupRejectTwo(q CorrelatedSetupRejectQuant) bool {
	if q == CorrelatedSetupRejectB {
		_ = q
	}
	if q == CorrelatedSetupRejectB {
		return false
	}
	return true
}

func CorrelatedSetupRejectQMatMul(q CorrelatedSetupRejectQuant) bool {
	return correlatedSetupRejectOne(q) || correlatedSetupRejectTwo(q)
}

// Correlation is disabled when the first matching body mutates the guarded
// enum variable before the repeated expression is evaluated.
type StableMutationQuant uint8

const (
	StableMutationA StableMutationQuant = iota // want `quant variant StableMutationA \(StableMutationQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	StableMutationB                            // want `quant variant StableMutationB \(StableMutationQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	StableMutationC                            // want `quant variant StableMutationC \(StableMutationQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
)

func stableMutationByteSize(q StableMutationQuant) int {
	switch q {
	case StableMutationA, StableMutationB, StableMutationC:
		return 8
	default:
		return 0
	}
}
func stableMutationDecode(q StableMutationQuant) []float32 {
	switch q {
	case StableMutationA, StableMutationB, StableMutationC:
		return []float32{}
	default:
		return nil
	}
}
func stableMutationOne(q StableMutationQuant) bool {
	if q == StableMutationB {
		q = StableMutationA
	}
	if q == StableMutationB {
		return false
	}
	return true
}
func (q *StableMutationQuant) resetToA() { *q = StableMutationA }
func stableMutationTwo(q StableMutationQuant) bool {
	if q == StableMutationB {
		q.resetToA()
	}
	if q == StableMutationB {
		return false
	}
	return true
}
func StableMutationQMatMul(q StableMutationQuant) bool {
	return stableMutationOne(q) || stableMutationTwo(q)
}

// A mutually exclusive arm can be skipped before matching a repeated guard.
type ElseIfCorrelationQuant uint8

const (
	ElseIfCorrelationA ElseIfCorrelationQuant = iota // want `quant variant ElseIfCorrelationA \(ElseIfCorrelationQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	ElseIfCorrelationB                               // want `quant variant ElseIfCorrelationB \(ElseIfCorrelationQuant\).*absent from 4 of 4 reachable CPU matmul dispatch layers`
	ElseIfCorrelationC                               // want `quant variant ElseIfCorrelationC \(ElseIfCorrelationQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
)

func elseIfCorrelationByteSize(q ElseIfCorrelationQuant) int {
	switch q {
	case ElseIfCorrelationA, ElseIfCorrelationB, ElseIfCorrelationC:
		return 8
	default:
		return 0
	}
}
func elseIfCorrelationDecode(q ElseIfCorrelationQuant) []float32 {
	switch q {
	case ElseIfCorrelationA, ElseIfCorrelationB, ElseIfCorrelationC:
		return []float32{}
	default:
		return nil
	}
}
func elseIfCorrelationOne(q ElseIfCorrelationQuant) bool {
	if q == ElseIfCorrelationB {
		_ = q
	}
	if q == ElseIfCorrelationA {
		return true
	} else if q == ElseIfCorrelationB {
		return false
	}
	return true
}
func elseIfCorrelationTwo(q ElseIfCorrelationQuant) bool {
	if q == ElseIfCorrelationB {
		_ = q
	}
	if q == ElseIfCorrelationA {
		return true
	} else if q == ElseIfCorrelationB {
		return false
	}
	return true
}
func ElseIfCorrelationQMatMul(q ElseIfCorrelationQuant) bool {
	return elseIfCorrelationOne(q) || elseIfCorrelationTwo(q)
}

// Unreachable statements after a non-returning call cannot create an implicit
// successful falloff in a void function.
type UnreachableVoidQuant uint8

const (
	UnreachableVoidA UnreachableVoidQuant = iota // want `quant variant UnreachableVoidA \(UnreachableVoidQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	UnreachableVoidB                             // want `quant variant UnreachableVoidB \(UnreachableVoidQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	UnreachableVoidC                             // want `quant variant UnreachableVoidC \(UnreachableVoidQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
)

func unreachableVoidByteSize(q UnreachableVoidQuant) int {
	switch q {
	case UnreachableVoidA, UnreachableVoidB, UnreachableVoidC:
		return 8
	default:
		return 0
	}
}
func unreachableVoidDecode(q UnreachableVoidQuant) []float32 {
	switch q {
	case UnreachableVoidA, UnreachableVoidB, UnreachableVoidC:
		return []float32{}
	default:
		return nil
	}
}
func unreachableVoidOne(q UnreachableVoidQuant) {
	if q == UnreachableVoidB {
		_ = q
	}
	panic("failed")
	_ = q
}
func unreachableVoidTwo(q UnreachableVoidQuant) {
	if q == UnreachableVoidB {
		_ = q
	}
	panic("failed")
	_ = q
}
func UnreachableVoidQMatMul(q UnreachableVoidQuant) {
	unreachableVoidOne(q)
	unreachableVoidTwo(q)
}

// A branch that exits the guarded body bypasses its lexical continuation.
type EscapingGuardBranchQuant uint8

const (
	EscapingGuardBranchA EscapingGuardBranchQuant = iota
	EscapingGuardBranchB
	EscapingGuardBranchC
)

func escapingGuardBranchByteSize(q EscapingGuardBranchQuant) int {
	switch q {
	case EscapingGuardBranchA, EscapingGuardBranchB, EscapingGuardBranchC:
		return 8
	default:
		return 0
	}
}
func escapingGuardBranchDecode(q EscapingGuardBranchQuant) []float32 {
	switch q {
	case EscapingGuardBranchA, EscapingGuardBranchB, EscapingGuardBranchC:
		return []float32{}
	default:
		return nil
	}
}
func escapingGuardBranchOne(q EscapingGuardBranchQuant) bool {
	for {
		if q == EscapingGuardBranchA {
			break
		}
		if q == EscapingGuardBranchB {
			return true
		}
		return false
	}
	return true
}
func escapingGuardBranchTwo(q EscapingGuardBranchQuant) bool {
	for {
		if q == EscapingGuardBranchA {
			break
		}
		if q == EscapingGuardBranchB {
			return true
		}
		return false
	}
	return true
}
func EscapingGuardBranchQMatMul(q EscapingGuardBranchQuant) bool {
	return escapingGuardBranchOne(q) || escapingGuardBranchTwo(q)
}

// A non-escaping branch can still prevent a matching guard body from ever
// reaching its successful lexical continuation.
type InfiniteGuardQuant uint8

const (
	InfiniteGuardA InfiniteGuardQuant = iota
	InfiniteGuardB                    // want `quant variant InfiniteGuardB \(InfiniteGuardQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	InfiniteGuardC
)

func infiniteGuardByteSize(q InfiniteGuardQuant) int {
	switch q {
	case InfiniteGuardA, InfiniteGuardB, InfiniteGuardC:
		return 8
	default:
		return 0
	}
}
func infiniteGuardDecode(q InfiniteGuardQuant) []float32 {
	switch q {
	case InfiniteGuardA, InfiniteGuardB, InfiniteGuardC:
		return []float32{}
	default:
		return nil
	}
}
func infiniteGuardOne(q InfiniteGuardQuant) bool {
	if q == InfiniteGuardB {
		for {
			continue
		}
	}
	return true
}
func infiniteGuardTwo(q InfiniteGuardQuant) bool {
	if q == InfiniteGuardB {
		select {}
	}
	return true
}
func InfiniteGuardQMatMul(q InfiniteGuardQuant) bool {
	return infiniteGuardOne(q) || infiniteGuardTwo(q)
}

// An internal break in a void continuation still reaches the implicit
// successful function return.
type InternalBranchVoidQuant uint8

const (
	InternalBranchVoidA InternalBranchVoidQuant = iota // want `quant variant InternalBranchVoidA \(InternalBranchVoidQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	InternalBranchVoidB
	InternalBranchVoidC // want `quant variant InternalBranchVoidC \(InternalBranchVoidQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
)

func internalBranchVoidByteSize(q InternalBranchVoidQuant) int {
	switch q {
	case InternalBranchVoidA, InternalBranchVoidB, InternalBranchVoidC:
		return 8
	default:
		return 0
	}
}
func internalBranchVoidDecode(q InternalBranchVoidQuant) []float32 {
	switch q {
	case InternalBranchVoidA, InternalBranchVoidB, InternalBranchVoidC:
		return []float32{}
	default:
		return nil
	}
}
func internalBranchVoidOne(q InternalBranchVoidQuant) {
	if q == InternalBranchVoidB {
		_ = q
	}
	for {
		break
	}
}
func internalBranchVoidTwo(q InternalBranchVoidQuant) {
	if q == InternalBranchVoidB {
		_ = q
	}
	for {
		break
	}
}
func InternalBranchVoidQMatMul(q InternalBranchVoidQuant) {
	internalBranchVoidOne(q)
	internalBranchVoidTwo(q)
}

// Open storage coverage contributes every declared enum value except its
// explicit exclusion to storage/decode eligibility.
type OpenEligibilityQuant uint8

const (
	OpenEligibilityA OpenEligibilityQuant = iota
	OpenEligibilityB                      // want `quant variant OpenEligibilityB \(OpenEligibilityQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	OpenEligibilityC
	OpenEligibilityArchiveOnly
)

const openEligibilityArchiveAlias = OpenEligibilityArchiveOnly

func openEligibilityByteSize(q OpenEligibilityQuant) int {
	if q != openEligibilityArchiveAlias {
		return 8
	}
	return 0
}
func openEligibilityDecode(q OpenEligibilityQuant) []float32 {
	switch q {
	case OpenEligibilityA, OpenEligibilityB, OpenEligibilityC:
		return []float32{}
	default:
		return nil
	}
}
func openEligibilityOne(q OpenEligibilityQuant) bool {
	switch q {
	case OpenEligibilityA, OpenEligibilityC:
		return true
	default:
		return false
	}
}
func openEligibilityTwo(q OpenEligibilityQuant) bool {
	switch q {
	case OpenEligibilityA, OpenEligibilityC:
		return true
	default:
		return false
	}
}
func OpenEligibilityQMatMul(q OpenEligibilityQuant) bool {
	return openEligibilityOne(q) || openEligibilityTwo(q)
}

// Helpers reached only with a fixed enum value or inside one matching switch
// arm are specialized paths, not general dispatch layers for every variant.
type SpecializedCallQuant uint8

const (
	SpecializedCallA SpecializedCallQuant = iota
	SpecializedCallB
	SpecializedCallC
)

func specializedCallByteSize(q SpecializedCallQuant) int {
	switch q {
	case SpecializedCallA, SpecializedCallB, SpecializedCallC:
		return 8
	default:
		return 0
	}
}
func specializedCallDecode(q SpecializedCallQuant) []float32 {
	switch q {
	case SpecializedCallA, SpecializedCallB, SpecializedCallC:
		return []float32{}
	default:
		return nil
	}
}
func specializedConstantPartial(q SpecializedCallQuant) bool {
	switch q {
	case SpecializedCallA:
		return true
	default:
		return false
	}
}
func specializedBranchPartial(q SpecializedCallQuant) bool {
	switch q {
	case SpecializedCallB:
		return true
	default:
		return false
	}
}
func SpecializedCallQMatMulOne(q SpecializedCallQuant) bool {
	switch q {
	case SpecializedCallA:
		return specializedConstantPartial(SpecializedCallA)
	case SpecializedCallB, SpecializedCallC:
		return true
	default:
		return false
	}
}
func SpecializedCallQMatMulTwo(q SpecializedCallQuant) bool {
	switch q {
	case SpecializedCallB:
		return specializedBranchPartial(q)
	case SpecializedCallA, SpecializedCallC:
		return true
	default:
		return false
	}
}

// Specialized callsites contribute only their reachable enum values, and
// multiple callsites union those values instead of deleting the helper layer.
type ScopedUnionQuant uint8

const (
	ScopedUnionA ScopedUnionQuant = iota
	ScopedUnionB                  // want `quant variant ScopedUnionB \(ScopedUnionQuant\).*absent from 1 of 3 reachable CPU matmul dispatch layers`
	ScopedUnionC
)

func scopedUnionByteSize(q ScopedUnionQuant) int {
	if q == ScopedUnionA || q == ScopedUnionB || q == ScopedUnionC {
		return 8
	}
	return 0
}
func scopedUnionDecode(q ScopedUnionQuant) []float32 {
	if q == ScopedUnionA || q == ScopedUnionB || q == ScopedUnionC {
		return []float32{}
	}
	return nil
}
func scopedUnionPartial(q ScopedUnionQuant) bool {
	switch q {
	case ScopedUnionA:
		return true
	default:
		return false
	}
}
func ScopedUnionQMatMulOne(q ScopedUnionQuant) bool {
	switch q {
	case ScopedUnionA:
		return scopedUnionPartial(ScopedUnionA)
	case ScopedUnionB:
		return scopedUnionPartial(ScopedUnionB)
	case ScopedUnionC:
		return true
	default:
		return false
	}
}
func ScopedUnionQMatMulTwo(q ScopedUnionQuant) bool {
	switch q {
	case ScopedUnionA, ScopedUnionB, ScopedUnionC:
		return true
	default:
		return false
	}
}

// A fixed parameter does not narrow a second, independently dynamic enum.
type MultiParamFixedQuant uint8
type MultiParamDynamicQuant uint8

const MultiParamFixedA MultiParamFixedQuant = 0
const (
	MultiParamDynamicA MultiParamDynamicQuant = iota
	MultiParamDynamicB                        // want `quant variant MultiParamDynamicB \(MultiParamDynamicQuant\).*absent from 1 of 3 reachable CPU matmul dispatch layers`
	MultiParamDynamicC
)

func multiParamByteSize(q MultiParamDynamicQuant) int {
	if q == MultiParamDynamicA || q == MultiParamDynamicB || q == MultiParamDynamicC {
		return 8
	}
	return 0
}
func multiParamDecode(q MultiParamDynamicQuant) []float32 {
	if q == MultiParamDynamicA || q == MultiParamDynamicB || q == MultiParamDynamicC {
		return []float32{}
	}
	return nil
}
func multiParamPartial(_ MultiParamFixedQuant, q MultiParamDynamicQuant) bool {
	switch q {
	case MultiParamDynamicA, MultiParamDynamicC:
		return true
	default:
		return false
	}
}
func MultiParamQMatMulOne(q MultiParamDynamicQuant) bool {
	switch q {
	case MultiParamDynamicA, MultiParamDynamicB, MultiParamDynamicC:
		return multiParamPartial(MultiParamFixedA, q)
	default:
		return false
	}
}
func MultiParamQMatMulTwo(q MultiParamDynamicQuant) bool {
	switch q {
	case MultiParamDynamicA, MultiParamDynamicB, MultiParamDynamicC:
		return true
	default:
		return false
	}
}

// A default-only arm is a general callsite, including calls from If.Init and
// from the left side of a short-circuit condition.
type GeneralContextQuant uint8

const (
	GeneralContextA GeneralContextQuant = iota
	GeneralContextB                     // want `quant variant GeneralContextB \(GeneralContextQuant\).*absent from 1 of 3 reachable CPU matmul dispatch layers`
	GeneralContextC                     // want `quant variant GeneralContextC \(GeneralContextQuant\).*absent from 1 of 3 reachable CPU matmul dispatch layers`
)

func generalContextByteSize(q GeneralContextQuant) int {
	if q == GeneralContextA || q == GeneralContextB || q == GeneralContextC {
		return 8
	}
	return 0
}
func generalContextDecode(q GeneralContextQuant) []float32 {
	if q == GeneralContextA || q == GeneralContextB || q == GeneralContextC {
		return []float32{}
	}
	return nil
}
func generalContextPartial(q GeneralContextQuant) bool {
	return q == GeneralContextA
}
func GeneralContextQMatMulOne(q GeneralContextQuant) bool {
	switch q {
	default:
		return generalContextPartial(q)
	}
}
func GeneralContextQMatMulTwo(q GeneralContextQuant) bool {
	if ok := generalContextPartial(q); ok && q == GeneralContextA {
		return true
	}
	return q == GeneralContextA || q == GeneralContextB || q == GeneralContextC
}
func GeneralContextQMatMulThree(q GeneralContextQuant) bool {
	return generalContextPartial(q) && q == GeneralContextA ||
		q == GeneralContextB || q == GeneralContextC
}

// Stable aliases, complement arms, invoked-literal callsites, and unexported
// matmul-named helpers retain their precise constrained domains.
type ScopedContextQuant uint8

const (
	ScopedContextA ScopedContextQuant = iota
	ScopedContextB                    // want `quant variant ScopedContextB \(ScopedContextQuant\).*absent from 1 of 5 reachable CPU matmul dispatch layers`
	ScopedContextC                    // want `quant variant ScopedContextC \(ScopedContextQuant\).*absent from 1 of 5 reachable CPU matmul dispatch layers`
)

func scopedContextByteSize(q ScopedContextQuant) int {
	if q == ScopedContextA || q == ScopedContextB || q == ScopedContextC {
		return 8
	}
	return 0
}
func scopedContextDecode(q ScopedContextQuant) []float32 {
	if q == ScopedContextA || q == ScopedContextB || q == ScopedContextC {
		return []float32{}
	}
	return nil
}
func scopedContextA(q ScopedContextQuant) bool { return q == ScopedContextA }
func scopedContextBC(q ScopedContextQuant) bool {
	return q == ScopedContextB || q == ScopedContextC
}
func scopedContextMatMul(q ScopedContextQuant) bool { return q == ScopedContextA }
func ScopedContextQMatMulOne(q ScopedContextQuant) bool {
	switch q {
	case ScopedContextA:
		alias := q
		return scopedContextA(alias) || scopedContextMatMul(alias)
	case ScopedContextB, ScopedContextC:
		return true
	default:
		return false
	}
}
func ScopedContextQMatMulTwo(q ScopedContextQuant) bool {
	if q == ScopedContextA {
		_ = 1
	} else {
		_ = scopedContextBC(q)
	}
	switch q {
	case ScopedContextA, ScopedContextB, ScopedContextC:
		return true
	default:
		return false
	}
}
func ScopedContextQMatMulThree(q ScopedContextQuant) bool {
	run := func() bool { return scopedContextA(q) }
	if q == ScopedContextA {
		return run()
	}
	return q == ScopedContextB || q == ScopedContextC
}

// Reach scopes attach to the callee parameter actually inspected, even when
// two parameters share the same enum type.
type SameParamQuant uint8

const (
	SameParamA SameParamQuant = iota
	SameParamB                // want `quant variant SameParamB \(SameParamQuant\).*absent from 1 of 3 reachable CPU matmul dispatch layers`
	SameParamC
)

func sameParamByteSize(q SameParamQuant) int {
	if q == SameParamA || q == SameParamB || q == SameParamC {
		return 8
	}
	return 0
}
func sameParamDecode(q SameParamQuant) []float32 {
	if q == SameParamA || q == SameParamB || q == SameParamC {
		return []float32{}
	}
	return nil
}
func sameParamHelper(fixed, dynamic SameParamQuant) bool {
	fixedOK := func() bool {
		switch fixed {
		case SameParamA:
			return true
		default:
			return false
		}
	}()
	dynamicOK := func() bool {
		switch dynamic {
		case SameParamA, SameParamC:
			return true
		default:
			return false
		}
	}()
	return fixedOK && dynamicOK
}
func SameParamQMatMulOne(q SameParamQuant) bool {
	switch q {
	case SameParamA, SameParamB, SameParamC:
		return sameParamHelper(SameParamA, q)
	default:
		return false
	}
}
func SameParamQMatMulTwo(q SameParamQuant) bool {
	switch q {
	case SameParamA, SameParamB, SameParamC:
		return true
	default:
		return false
	}
}

// Undeclared fixed values do not make a finite callsite union look universal.
type ExtraValueScopeQuant uint8

const (
	ExtraValueScopeA ExtraValueScopeQuant = iota
	ExtraValueScopeB
	ExtraValueScopeC
)

func extraValueScopeByteSize(q ExtraValueScopeQuant) int {
	if q == ExtraValueScopeA || q == ExtraValueScopeB || q == ExtraValueScopeC {
		return 8
	}
	return 0
}
func extraValueScopeDecode(q ExtraValueScopeQuant) []float32 {
	if q == ExtraValueScopeA || q == ExtraValueScopeB || q == ExtraValueScopeC {
		return []float32{}
	}
	return nil
}
func extraValueScopePartial(q ExtraValueScopeQuant) bool {
	return q == ExtraValueScopeA || q == ExtraValueScopeB
}
func ExtraValueScopeQMatMulOne(q ExtraValueScopeQuant) bool {
	_ = extraValueScopePartial(ExtraValueScopeA)
	_ = extraValueScopePartial(ExtraValueScopeB)
	_ = extraValueScopePartial(ExtraValueScopeQuant(99))
	return q == ExtraValueScopeA || q == ExtraValueScopeB || q == ExtraValueScopeC
}
func ExtraValueScopeQMatMulTwo(q ExtraValueScopeQuant) bool {
	return q == ExtraValueScopeA || q == ExtraValueScopeB || q == ExtraValueScopeC
}

// Captured locals are resolved at invocation time, and known single-target
// function aliases contribute ordinary scoped call edges.
type CapturedScopeQuant uint8

const (
	CapturedScopeA CapturedScopeQuant = iota
	CapturedScopeB
	CapturedScopeC
)

func capturedScopeByteSize(q CapturedScopeQuant) int {
	if q == CapturedScopeA || q == CapturedScopeB || q == CapturedScopeC {
		return 8
	}
	return 0
}
func capturedScopeDecode(q CapturedScopeQuant) []float32 {
	if q == CapturedScopeA || q == CapturedScopeB || q == CapturedScopeC {
		return []float32{}
	}
	return nil
}
func capturedScopeB(q CapturedScopeQuant) bool { return q == CapturedScopeB }
func CapturedScopeQMatMulOne(q CapturedScopeQuant) bool {
	value := CapturedScopeA
	run := func() bool { return capturedScopeB(value) }
	value = CapturedScopeB
	_ = run()
	return q == CapturedScopeA || q == CapturedScopeB || q == CapturedScopeC
}
func CapturedScopeQMatMulTwo(q CapturedScopeQuant) bool {
	return q == CapturedScopeA || q == CapturedScopeB || q == CapturedScopeC
}

type NamedAliasScopeQuant uint8

const (
	NamedAliasScopeA NamedAliasScopeQuant = iota
	NamedAliasScopeB                      // want `quant variant NamedAliasScopeB \(NamedAliasScopeQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	NamedAliasScopeC
)

func namedAliasScopeByteSize(q NamedAliasScopeQuant) int {
	if q == NamedAliasScopeA || q == NamedAliasScopeB || q == NamedAliasScopeC {
		return 8
	}
	return 0
}
func namedAliasScopeDecode(q NamedAliasScopeQuant) []float32 {
	if q == NamedAliasScopeA || q == NamedAliasScopeB || q == NamedAliasScopeC {
		return []float32{}
	}
	return nil
}
func namedAliasScopePartial(q NamedAliasScopeQuant) bool {
	return q == NamedAliasScopeA || q == NamedAliasScopeC
}
func NamedAliasScopeQMatMulOne(q NamedAliasScopeQuant) bool {
	fn := namedAliasScopePartial
	return fn(q)
}
func NamedAliasScopeQMatMulTwo(q NamedAliasScopeQuant) bool {
	return q == NamedAliasScopeA || q == NamedAliasScopeB || q == NamedAliasScopeC
}

// Named callbacks passed to a proven direct dispatcher retain their actual
// callback argument scopes instead of disappearing or becoming global roots.
type NamedCallbackScopeQuant uint8

const (
	NamedCallbackScopeA NamedCallbackScopeQuant = iota
	NamedCallbackScopeB                         // want `quant variant NamedCallbackScopeB \(NamedCallbackScopeQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	NamedCallbackScopeC
)

func namedCallbackScopeByteSize(q NamedCallbackScopeQuant) int {
	if q == NamedCallbackScopeA || q == NamedCallbackScopeB || q == NamedCallbackScopeC {
		return 8
	}
	return 0
}
func namedCallbackScopeDecode(q NamedCallbackScopeQuant) []float32 {
	if q == NamedCallbackScopeA || q == NamedCallbackScopeB || q == NamedCallbackScopeC {
		return []float32{}
	}
	return nil
}
func namedCallbackDispatcher(q NamedCallbackScopeQuant, callback func(NamedCallbackScopeQuant) bool) bool {
	return callback(q)
}
func namedCallbackMatMulA(q NamedCallbackScopeQuant) bool { return q == NamedCallbackScopeA }
func namedCallbackPartial(q NamedCallbackScopeQuant) bool {
	return q == NamedCallbackScopeA || q == NamedCallbackScopeC
}
func NamedCallbackScopeQMatMulOne(q NamedCallbackScopeQuant) bool {
	_ = namedCallbackDispatcher(NamedCallbackScopeA, namedCallbackMatMulA)
	return namedCallbackDispatcher(q, namedCallbackPartial)
}
func NamedCallbackScopeQMatMulTwo(q NamedCallbackScopeQuant) bool {
	return q == NamedCallbackScopeA || q == NamedCallbackScopeB || q == NamedCallbackScopeC
}

// Returns inside an invoked literal use the literal's signature rather than
// the enclosing bool function's signature when classifying success.
type LiteralSignatureQuant uint8

const (
	LiteralSignatureA LiteralSignatureQuant = iota
	LiteralSignatureB
	LiteralSignatureC
)

func literalSignatureByteSize(q LiteralSignatureQuant) int {
	switch q {
	case LiteralSignatureA, LiteralSignatureB, LiteralSignatureC:
		return 8
	default:
		return 0
	}
}

func literalSignatureDecode(q LiteralSignatureQuant) []float32 {
	switch q {
	case LiteralSignatureA, LiteralSignatureB, LiteralSignatureC:
		return []float32{}
	default:
		return nil
	}
}

func literalSignatureOne(q LiteralSignatureQuant) bool {
	_ = func() error {
		if q == LiteralSignatureB {
			_ = q
		}
		return nil
	}()
	return true
}

func literalSignatureTwo(q LiteralSignatureQuant) bool {
	_ = func() error {
		if q == LiteralSignatureB {
			_ = q
		}
		return nil
	}()
	return true
}

func LiteralSignatureQMatMul(q LiteralSignatureQuant) bool {
	return literalSignatureOne(q) && literalSignatureTwo(q)
}

// Sequential terminal guards are alternatives in one dispatch layer, not
// independent layers that can diagnose one another.
type SequentialQuant uint8

const (
	SequentialA SequentialQuant = iota
	SequentialB
)

func sequentialByteSize(q SequentialQuant) int {
	switch q {
	case SequentialA, SequentialB:
		return 8
	default:
		return 0
	}
}
func sequentialDecode(q SequentialQuant) []float32 {
	switch q {
	case SequentialA, SequentialB:
		return []float32{}
	default:
		return nil
	}
}
func SequentialQMatMul(q SequentialQuant) bool {
	if q == SequentialA {
		return true
	}
	if q == SequentialB {
		return true
	}
	return false
}

// Invoked literals use their own CFG, so unreachable returns in the literal do
// not manufacture another CPU dispatch layer.
type LiteralCFGQuant uint8

const (
	LiteralCFGA LiteralCFGQuant = iota
	LiteralCFGB                 // want `quant variant LiteralCFGB \(LiteralCFGQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	LiteralCFGC
)

func literalCFGByteSize(q LiteralCFGQuant) int {
	switch q {
	case LiteralCFGA, LiteralCFGB, LiteralCFGC:
		return 8
	default:
		return 0
	}
}
func literalCFGDecode(q LiteralCFGQuant) []float32 {
	switch q {
	case LiteralCFGA, LiteralCFGB, LiteralCFGC:
		return []float32{}
	default:
		return nil
	}
}

var literalCFGDeadTable = map[LiteralCFGQuant]func(){
	LiteralCFGA: func() {}, LiteralCFGB: func() {}, LiteralCFGC: func() {},
}

func LiteralCFGQMatMul(q LiteralCFGQuant) bool {
	return func() bool {
		table := map[LiteralCFGQuant]func(){LiteralCFGA: func() {}, LiteralCFGC: func() {}}
		if _, ok := table[q]; !ok {
			return false
		}
		return q == LiteralCFGA || q == LiteralCFGC
		switch q {
		case LiteralCFGA, LiteralCFGB, LiteralCFGC:
			return true
		}
		table[LiteralCFGB] = func() {}
		_ = table[q]
		_ = literalCFGDeadTable[q]
		return false
	}()
}

// A function variable overwritten before its call does not keep the stale
// literal as reachable dispatch evidence.
type LiteralRebindQuant uint8

const (
	LiteralRebindA LiteralRebindQuant = iota
	LiteralRebindB                    // want `quant variant LiteralRebindB \(LiteralRebindQuant\).*absent from 1 of 3 reachable CPU matmul dispatch layers`
	LiteralRebindC
)

func literalRebindByteSize(q LiteralRebindQuant) int {
	switch q {
	case LiteralRebindA, LiteralRebindB, LiteralRebindC:
		return 8
	default:
		return 0
	}
}
func literalRebindDecode(q LiteralRebindQuant) []float32 {
	switch q {
	case LiteralRebindA, LiteralRebindB, LiteralRebindC:
		return []float32{}
	default:
		return nil
	}
}
func literalRebindAll(q LiteralRebindQuant) bool {
	return q == LiteralRebindA || q == LiteralRebindB || q == LiteralRebindC
}
func literalRebindOtherLayer(q LiteralRebindQuant) bool {
	return q == LiteralRebindA || q == LiteralRebindB || q == LiteralRebindC
}
func literalRebindLastPartial(q LiteralRebindQuant) bool {
	dispatch := func(value LiteralRebindQuant) bool {
		return value == LiteralRebindA || value == LiteralRebindB || value == LiteralRebindC
	}
	dispatch = func(value LiteralRebindQuant) bool {
		return value == LiteralRebindA || value == LiteralRebindC
	}
	return dispatch(q)
}
func LiteralRebindQMatMul(q LiteralRebindQuant) bool {
	dispatch := func(value LiteralRebindQuant) bool {
		return value == LiteralRebindA || value == LiteralRebindC
	}
	dispatch = literalRebindAll
	return dispatch(q) && literalRebindLastPartial(q) && literalRebindOtherLayer(q)
}

// A captured function variable is resolved at the point where its enclosing
// literal executes, not at the earlier source position of the captured call.
type CapturedBindingQuant uint8

const (
	CapturedBindingA CapturedBindingQuant = iota
	CapturedBindingB                      // want `quant variant CapturedBindingB \(CapturedBindingQuant\).*absent from 3 of 3 reachable CPU matmul dispatch layers`
	CapturedBindingC
)

func capturedBindingByteSize(q CapturedBindingQuant) int {
	switch q {
	case CapturedBindingA, CapturedBindingB, CapturedBindingC:
		return 8
	default:
		return 0
	}
}
func capturedBindingDecode(q CapturedBindingQuant) []float32 {
	switch q {
	case CapturedBindingA, CapturedBindingB, CapturedBindingC:
		return []float32{}
	default:
		return nil
	}
}
func CapturedBindingQMatMulOne(q CapturedBindingQuant) bool {
	var dispatch func() bool
	run := func() bool { return dispatch() }
	dispatch = func() bool { return q == CapturedBindingA || q == CapturedBindingC }
	return run()
}
func CapturedBindingQMatMulTwo(q CapturedBindingQuant) bool {
	var dispatch func() bool
	inner := func() bool { return dispatch() }
	run := func() bool { return inner() }
	dispatch = func() bool { return q == CapturedBindingA || q == CapturedBindingC }
	return run()
}
func CapturedBindingQMatMulThree(q CapturedBindingQuant) bool {
	dispatch := func() bool {
		return q == CapturedBindingA || q == CapturedBindingB || q == CapturedBindingC
	}
	run := func() bool {
		dispatch = func() bool { return q == CapturedBindingA || q == CapturedBindingC }
		return dispatch()
	}
	return run()
}

// A conditionally rejecting case still supports its variants through a
// successful common continuation.
type ConditionalCaseQuant uint8

const (
	ConditionalCaseA ConditionalCaseQuant = iota
	ConditionalCaseB
)

func conditionalCaseByteSize(q ConditionalCaseQuant) int {
	switch q {
	case ConditionalCaseA, ConditionalCaseB:
		return 8
	default:
		return 0
	}
}
func conditionalCaseDecode(q ConditionalCaseQuant) []float32 {
	switch q {
	case ConditionalCaseA, ConditionalCaseB:
		return []float32{}
	default:
		return nil
	}
}
func conditionalCaseOne(q ConditionalCaseQuant, rows int) bool {
	switch q {
	case ConditionalCaseA, ConditionalCaseB:
		if rows == 0 {
			return false
		}
	default:
		return false
	}
	return true
}
func conditionalCaseTwo(q ConditionalCaseQuant, rows int) bool {
	switch q {
	case ConditionalCaseA, ConditionalCaseB:
		if rows == 0 {
			return false
		}
	default:
		return false
	}
	return true
}
func conditionalCaseSuccess(q ConditionalCaseQuant, rows int) bool {
	switch q {
	case ConditionalCaseA, ConditionalCaseB:
		if rows == 1 {
			return true
		}
	default:
		return false
	}
	return false
}
func conditionalCaseEmpty(q ConditionalCaseQuant) bool {
	switch q {
	case ConditionalCaseA:
	case ConditionalCaseB:
		return true
	}
	return true
}
func conditionalCaseImplicitDefault(q ConditionalCaseQuant) bool {
	switch q {
	case ConditionalCaseA:
		return true
	}
	return true
}
func ConditionalCaseQMatMul(q ConditionalCaseQuant, rows int) bool {
	return conditionalCaseOne(q, rows) && conditionalCaseTwo(q, rows) &&
		conditionalCaseSuccess(q, rows) && conditionalCaseEmpty(q) && conditionalCaseImplicitDefault(q)
}

// Sequential guards in separate invoked literals remain independent layers.
type LiteralLayersQuant uint8

const (
	LiteralLayersA LiteralLayersQuant = iota
	LiteralLayersB                    // want `quant variant LiteralLayersB \(LiteralLayersQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	LiteralLayersC
)

func literalLayersByteSize(q LiteralLayersQuant) int {
	switch q {
	case LiteralLayersA, LiteralLayersB, LiteralLayersC:
		return 8
	default:
		return 0
	}
}
func literalLayersDecode(q LiteralLayersQuant) []float32 {
	switch q {
	case LiteralLayersA, LiteralLayersB, LiteralLayersC:
		return []float32{}
	default:
		return nil
	}
}
func LiteralLayersQMatMul(q LiteralLayersQuant) bool {
	first := func() bool {
		if q == LiteralLayersA || q == LiteralLayersC {
			return true
		}
		return false
	}
	second := func() bool {
		if q == LiteralLayersA || q == LiteralLayersC {
			return true
		}
		return false
	}
	return first() && second()
}

// Mutually exclusive returns in one function are alternatives in one layer.
type BranchReturnQuant uint8

const (
	BranchReturnA BranchReturnQuant = iota
	BranchReturnB
	BranchReturnC
)

func branchReturnByteSize(q BranchReturnQuant) int {
	switch q {
	case BranchReturnA, BranchReturnB, BranchReturnC:
		return 8
	default:
		return 0
	}
}
func branchReturnDecode(q BranchReturnQuant) []float32 {
	switch q {
	case BranchReturnA, BranchReturnB, BranchReturnC:
		return []float32{}
	default:
		return nil
	}
}
func BranchReturnQMatMul(q BranchReturnQuant, fast bool) bool {
	if fast {
		return q == BranchReturnA || q == BranchReturnC
	}
	return q == BranchReturnA || q == BranchReturnC
}

// A successful guard and its final return are alternatives in one dispatch
// decision, not two independent layers.
type MixedAlternativeQuant uint8

const (
	MixedAlternativeA MixedAlternativeQuant = iota
	MixedAlternativeB
	MixedAlternativeC
)

func mixedAlternativeByteSize(q MixedAlternativeQuant) int {
	switch q {
	case MixedAlternativeA, MixedAlternativeB, MixedAlternativeC:
		return 8
	default:
		return 0
	}
}
func mixedAlternativeDecode(q MixedAlternativeQuant) []float32 {
	switch q {
	case MixedAlternativeA, MixedAlternativeB, MixedAlternativeC:
		return []float32{}
	default:
		return nil
	}
}
func MixedAlternativeQMatMul(q MixedAlternativeQuant) bool {
	if q == MixedAlternativeA {
		return true
	}
	return q == MixedAlternativeC
}

// A successful switch case and its common final return are likewise one
// alternative dispatch decision.
type SwitchAlternativeQuant uint8

const (
	SwitchAlternativeA SwitchAlternativeQuant = iota
	SwitchAlternativeB
	SwitchAlternativeC
)

func switchAlternativeByteSize(q SwitchAlternativeQuant) int {
	switch q {
	case SwitchAlternativeA, SwitchAlternativeB, SwitchAlternativeC:
		return 8
	default:
		return 0
	}
}
func switchAlternativeDecode(q SwitchAlternativeQuant) []float32 {
	switch q {
	case SwitchAlternativeA, SwitchAlternativeB, SwitchAlternativeC:
		return []float32{}
	default:
		return nil
	}
}
func SwitchAlternativeQMatMul(q SwitchAlternativeQuant) bool {
	switch q {
	case SwitchAlternativeA:
		return true
	}
	return q == SwitchAlternativeC
}

// A literal bound in an outer lexical block remains visible when the call is
// nested inside a reachable conditional block.
type OuterLiteralQuant uint8

const (
	OuterLiteralA OuterLiteralQuant = iota
	OuterLiteralB                   // want `quant variant OuterLiteralB \(OuterLiteralQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	OuterLiteralC
)

func outerLiteralByteSize(q OuterLiteralQuant) int {
	switch q {
	case OuterLiteralA, OuterLiteralB, OuterLiteralC:
		return 8
	default:
		return 0
	}
}
func outerLiteralDecode(q OuterLiteralQuant) []float32 {
	switch q {
	case OuterLiteralA, OuterLiteralB, OuterLiteralC:
		return []float32{}
	default:
		return nil
	}
}
func outerLiteralNamed(q OuterLiteralQuant) bool {
	return q == OuterLiteralA || q == OuterLiteralC
}
func OuterLiteralQMatMul(q OuterLiteralQuant, enabled bool) bool {
	dispatch := func() bool {
		return q == OuterLiteralA || q == OuterLiteralC
	}
	if enabled {
		return dispatch() && outerLiteralNamed(q)
	}
	return false
}

// Bare returns from void dispatch helpers are successful handled exits; the
// explicit panic fallback still rejects the omitted variant.
type VoidDispatchQuant uint8

const (
	VoidDispatchA VoidDispatchQuant = iota
	VoidDispatchB                   // want `quant variant VoidDispatchB \(VoidDispatchQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	VoidDispatchC
)

func voidDispatchByteSize(q VoidDispatchQuant) int {
	switch q {
	case VoidDispatchA, VoidDispatchB, VoidDispatchC:
		return 8
	default:
		return 0
	}
}
func voidDispatchDecode(q VoidDispatchQuant) []float32 {
	switch q {
	case VoidDispatchA, VoidDispatchB, VoidDispatchC:
		return []float32{}
	default:
		return nil
	}
}
func VoidDispatchQMatMulOne(q VoidDispatchQuant) {
	switch q {
	case VoidDispatchA, VoidDispatchC:
		return
	default:
		panic("unsupported")
	}
}
func handleVoidDispatch() {}
func VoidDispatchQMatMulTwo(q VoidDispatchQuant) {
	switch q {
	case VoidDispatchA, VoidDispatchC:
		handleVoidDispatch()
	default:
		panic("unsupported")
	}
}

// A local error variable is not an unsupported result merely because its name
// begins with err. Only the explicit constructor in the fallback rejects.
type ErrVariableQuant uint8

const (
	ErrVariableA ErrVariableQuant = iota
	ErrVariableB
	ErrVariableC
)

func errVariableByteSize(q ErrVariableQuant) int {
	switch q {
	case ErrVariableA, ErrVariableB, ErrVariableC:
		return 8
	default:
		return 0
	}
}
func errVariableDecode(q ErrVariableQuant) []float32 {
	switch q {
	case ErrVariableA, ErrVariableB, ErrVariableC:
		return []float32{}
	default:
		return nil
	}
}
func runErrVariableKernel(ErrVariableQuant) error { return nil }
func ErrVariableQMatMulOne(q ErrVariableQuant) error {
	switch q {
	case ErrVariableA, ErrVariableB, ErrVariableC:
		err := runErrVariableKernel(q)
		return err
	default:
		return errors.New("unsupported")
	}
}
func ErrVariableQMatMulTwo(q ErrVariableQuant) error {
	switch q {
	case ErrVariableA, ErrVariableB, ErrVariableC:
		errKernel := runErrVariableKernel(q)
		return errKernel
	default:
		return errors.New("unsupported")
	}
}

// A predicate assigned to a named result narrows a multi-value case before a
// bare return, just like the same predicate in an explicit return expression.
type NamedResultPredicateQuant uint8

const (
	NamedResultPredicateA NamedResultPredicateQuant = iota
	NamedResultPredicateB                           // want `quant variant NamedResultPredicateB \(NamedResultPredicateQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	NamedResultPredicateC
)

func namedResultPredicateByteSize(q NamedResultPredicateQuant) int {
	switch q {
	case NamedResultPredicateA, NamedResultPredicateB, NamedResultPredicateC:
		return 8
	default:
		return 0
	}
}
func namedResultPredicateDecode(q NamedResultPredicateQuant) []float32 {
	switch q {
	case NamedResultPredicateA, NamedResultPredicateB, NamedResultPredicateC:
		return []float32{}
	default:
		return nil
	}
}
func NamedResultPredicateQMatMulOne(q NamedResultPredicateQuant) (ok bool) {
	switch q {
	case NamedResultPredicateA, NamedResultPredicateB:
		ok = q == NamedResultPredicateA
		return
	case NamedResultPredicateC:
		ok = true
		return
	default:
		return
	}
}
func NamedResultPredicateQMatMulTwo(q NamedResultPredicateQuant) (ok bool) {
	switch q {
	case NamedResultPredicateA, NamedResultPredicateB:
		ok = q != NamedResultPredicateB
		return
	case NamedResultPredicateC:
		ok = true
		return
	default:
		return
	}
}

// Assignment-form range variables write named results only when the range can
// iterate. A one-element range supports B; a statically empty range preserves
// the initial false result and therefore rejects B.
type NamedRangeWriteQuant uint8

const (
	NamedRangeWriteA NamedRangeWriteQuant = iota
	NamedRangeWriteB
	NamedRangeWriteC
)

func namedRangeWriteByteSize(q NamedRangeWriteQuant) int {
	switch q {
	case NamedRangeWriteA, NamedRangeWriteB, NamedRangeWriteC:
		return 8
	default:
		return 0
	}
}
func namedRangeWriteDecode(q NamedRangeWriteQuant) []float32 {
	switch q {
	case NamedRangeWriteA, NamedRangeWriteB, NamedRangeWriteC:
		return []float32{}
	default:
		return nil
	}
}
func NamedRangeWriteQMatMulOne(q NamedRangeWriteQuant) (ok bool) {
	switch q {
	case NamedRangeWriteA, NamedRangeWriteC:
		return true
	case NamedRangeWriteB:
		for _, ok = range []bool{true} {
		}
		return
	default:
		return false
	}
}
func NamedRangeWriteQMatMulTwo(q NamedRangeWriteQuant) (ok bool) {
	switch q {
	case NamedRangeWriteA, NamedRangeWriteC:
		return true
	case NamedRangeWriteB:
		for _, ok = range []bool{true} {
		}
		return
	default:
		return false
	}
}

type EmptyNamedRangeWriteQuant uint8

const (
	EmptyNamedRangeWriteA EmptyNamedRangeWriteQuant = iota
	EmptyNamedRangeWriteB                           // want `quant variant EmptyNamedRangeWriteB \(EmptyNamedRangeWriteQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	EmptyNamedRangeWriteC
)

func emptyNamedRangeWriteByteSize(q EmptyNamedRangeWriteQuant) int {
	switch q {
	case EmptyNamedRangeWriteA, EmptyNamedRangeWriteB, EmptyNamedRangeWriteC:
		return 8
	default:
		return 0
	}
}
func emptyNamedRangeWriteDecode(q EmptyNamedRangeWriteQuant) []float32 {
	switch q {
	case EmptyNamedRangeWriteA, EmptyNamedRangeWriteB, EmptyNamedRangeWriteC:
		return []float32{}
	default:
		return nil
	}
}
func EmptyNamedRangeWriteQMatMulOne(q EmptyNamedRangeWriteQuant) (ok bool) {
	switch q {
	case EmptyNamedRangeWriteA, EmptyNamedRangeWriteC:
		return true
	case EmptyNamedRangeWriteB:
		for _, ok = range [0]bool{} {
		}
		return
	default:
		return false
	}
}
func EmptyNamedRangeWriteQMatMulTwo(q EmptyNamedRangeWriteQuant) (ok bool) {
	switch q {
	case EmptyNamedRangeWriteA, EmptyNamedRangeWriteC:
		return true
	case EmptyNamedRangeWriteB:
		for _, ok = range [0]bool{} {
		}
		return
	default:
		return false
	}
}

// A separately successful result keeps the whole multi-result return handled.
type MultiResultSuccessQuant uint8

const (
	MultiResultSuccessA MultiResultSuccessQuant = iota
	MultiResultSuccessB
	MultiResultSuccessC
)

func multiResultSuccessByteSize(q MultiResultSuccessQuant) int {
	switch q {
	case MultiResultSuccessA, MultiResultSuccessB, MultiResultSuccessC:
		return 8
	default:
		return 0
	}
}
func multiResultSuccessDecode(q MultiResultSuccessQuant) []float32 {
	switch q {
	case MultiResultSuccessA, MultiResultSuccessB, MultiResultSuccessC:
		return []float32{}
	default:
		return nil
	}
}
func MultiResultSuccessQMatMulOne(q MultiResultSuccessQuant) (bool, error) {
	switch q {
	case MultiResultSuccessA, MultiResultSuccessB:
		return q == MultiResultSuccessA, nil
	case MultiResultSuccessC:
		return true, nil
	default:
		return false, errors.New("unsupported")
	}
}
func MultiResultSuccessQMatMulTwo(q MultiResultSuccessQuant) (bool, error) {
	switch q {
	case MultiResultSuccessA, MultiResultSuccessB:
		return q != MultiResultSuccessB, nil
	case MultiResultSuccessC:
		return true, nil
	default:
		return false, errors.New("unsupported")
	}
}

// Tuple-valued forwarding returns preserve every logical result. A false
// predicate paired with a proven error rejects the forwarded case, while a
// true predicate paired with nil remains a real support inverse.
type ForwardedTupleReturnQuant uint8

const (
	ForwardedTupleReturnA ForwardedTupleReturnQuant = iota
	ForwardedTupleReturnB                           // want `quant variant ForwardedTupleReturnB \(ForwardedTupleReturnQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	ForwardedTupleReturnC
)

func forwardedTupleReturnByteSize(q ForwardedTupleReturnQuant) int {
	switch q {
	case ForwardedTupleReturnA, ForwardedTupleReturnB, ForwardedTupleReturnC:
		return 8
	default:
		return 0
	}
}
func forwardedTupleReturnDecode(q ForwardedTupleReturnQuant) []float32 {
	switch q {
	case ForwardedTupleReturnA, ForwardedTupleReturnB, ForwardedTupleReturnC:
		return []float32{}
	default:
		return nil
	}
}
func forwardedTupleReturnReject() (bool, error) {
	return false, errors.New("unsupported")
}
func forwardedTupleReturnRejectAgain() (bool, error) {
	return forwardedTupleReturnReject()
}
func forwardedTupleReturnSupport() (bool, error) {
	return true, nil
}
func forwardedTupleNamedBareReject() (ok bool, err error) {
	ok, err = forwardedTupleReturnRejectAgain()
	return
}
func forwardedTupleNamedBareSupport() (ok bool, err error) {
	ok, err = forwardedTupleReturnSupport()
	return
}
func forwardedTupleNamedExplicitReject() (ok bool, err error) {
	ok = false
	err = errors.New("unsupported")
	return ok, err
}
func forwardedTupleNamedExplicitSupport() (ok bool, err error) {
	ok = true
	err = nil
	return ok, err
}
func ForwardedTupleReturnQMatMulOne(q ForwardedTupleReturnQuant) (bool, error) {
	switch q {
	case ForwardedTupleReturnA, ForwardedTupleReturnC:
		return forwardedTupleNamedBareSupport()
	case ForwardedTupleReturnB:
		return forwardedTupleNamedBareReject()
	default:
		return false, errors.New("unsupported")
	}
}
func ForwardedTupleReturnQMatMulTwo(q ForwardedTupleReturnQuant) (bool, error) {
	switch q {
	case ForwardedTupleReturnA, ForwardedTupleReturnC:
		return forwardedTupleNamedExplicitSupport()
	case ForwardedTupleReturnB:
		return forwardedTupleNamedExplicitReject()
	default:
		return false, errors.New("unsupported")
	}
}

// Tuple-valued support summaries retain complements through both if/else and
// following-return shapes.
type TupleFallbackQuant uint8

const (
	TupleFallbackA TupleFallbackQuant = iota
	TupleFallbackB                    // want `quant variant TupleFallbackB \(TupleFallbackQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	TupleFallbackC
)

func tupleFallbackByteSize(q TupleFallbackQuant) int {
	switch q {
	case TupleFallbackA, TupleFallbackB, TupleFallbackC:
		return 8
	default:
		return 0
	}
}
func tupleFallbackDecode(q TupleFallbackQuant) []float32 {
	switch q {
	case TupleFallbackA, TupleFallbackB, TupleFallbackC:
		return []float32{}
	default:
		return nil
	}
}
func TupleFallbackQMatMulOne(q TupleFallbackQuant) (bool, error) {
	if q == TupleFallbackB {
		return forwardedTupleReturnReject()
	} else {
		return forwardedTupleReturnSupport()
	}
}
func TupleFallbackQMatMulTwo(q TupleFallbackQuant) (bool, error) {
	if q == TupleFallbackB {
		return forwardedTupleReturnRejectAgain()
	}
	return forwardedTupleReturnSupport()
}

// A helper with mixed success and failure returns is neither always supporting
// nor always rejecting, so callers remain conservative.
type TupleMixedReturnQuant uint8

const (
	TupleMixedReturnA TupleMixedReturnQuant = iota
	TupleMixedReturnB
	TupleMixedReturnC
)

func tupleMixedReturnByteSize(q TupleMixedReturnQuant) int {
	switch q {
	case TupleMixedReturnA, TupleMixedReturnB, TupleMixedReturnC:
		return 8
	default:
		return 0
	}
}
func tupleMixedReturnDecode(q TupleMixedReturnQuant) []float32 {
	switch q {
	case TupleMixedReturnA, TupleMixedReturnB, TupleMixedReturnC:
		return []float32{}
	default:
		return nil
	}
}
func tupleMixedReturn(value bool) (bool, error) {
	if value {
		return true, nil
	}
	return false, errors.New("unsupported")
}
func TupleMixedReturnQMatMulOne(q TupleMixedReturnQuant) (bool, error) {
	return tupleMixedReturn(q != TupleMixedReturnB)
}
func TupleMixedReturnQMatMulTwo(q TupleMixedReturnQuant) (bool, error) {
	return tupleMixedReturn(q == TupleMixedReturnA || q == TupleMixedReturnC)
}

// A proven non-nil error rejects a return even when an earlier result is
// dynamic and could otherwise look successful.
type DominantErrorQuant uint8

const (
	DominantErrorA DominantErrorQuant = iota
	DominantErrorB                    // want `quant variant DominantErrorB \(DominantErrorQuant\).*absent from 3 of 3 reachable CPU matmul dispatch layers`
	DominantErrorC
)

func dominantErrorByteSize(q DominantErrorQuant) int {
	switch q {
	case DominantErrorA, DominantErrorB, DominantErrorC:
		return 8
	default:
		return 0
	}
}
func dominantErrorDecode(q DominantErrorQuant) []float32 {
	switch q {
	case DominantErrorA, DominantErrorB, DominantErrorC:
		return []float32{}
	default:
		return nil
	}
}
func DominantErrorQMatMulOne(q DominantErrorQuant) (int, error) {
	switch q {
	case DominantErrorA, DominantErrorC:
		return 1, nil
	default:
		return int(q), errors.New("unsupported")
	}
}
func DominantErrorQMatMulTwo(q DominantErrorQuant) (int, error) {
	switch q {
	case DominantErrorA, DominantErrorC:
		return 1, nil
	default:
		return int(q), errors.New("unsupported")
	}
}
func DominantErrorQMatMulThree(q DominantErrorQuant) (value int, err error) {
	switch q {
	case DominantErrorA, DominantErrorC:
		value = 1
		return
	default:
		value = int(q)
		err = errors.New("unsupported")
		return
	}
}

// Reaching definitions for named results apply across switch clauses, even
// when the clause contains unrelated statements and returns the names
// explicitly.
type ReachingErrorQuant uint8

const (
	ReachingErrorA ReachingErrorQuant = iota // want `quant variant ReachingErrorA \(ReachingErrorQuant\).*absent from 4 of 4 reachable CPU matmul dispatch layers`
	ReachingErrorB                           // want `quant variant ReachingErrorB \(ReachingErrorQuant\).*absent from 4 of 4 reachable CPU matmul dispatch layers`
	ReachingErrorC                           // want `quant variant ReachingErrorC \(ReachingErrorQuant\).*absent from 4 of 4 reachable CPU matmul dispatch layers`
)

func reachingErrorByteSize(q ReachingErrorQuant) int {
	switch q {
	case ReachingErrorA, ReachingErrorB, ReachingErrorC:
		return 8
	default:
		return 0
	}
}
func reachingErrorDecode(q ReachingErrorQuant) []float32 {
	switch q {
	case ReachingErrorA, ReachingErrorB, ReachingErrorC:
		return []float32{}
	default:
		return nil
	}
}
func ReachingErrorQMatMulOne(q ReachingErrorQuant) (ok bool, err error) {
	err = errors.New("unsupported")
	switch q {
	case ReachingErrorA, ReachingErrorC:
		println("side effect")
		return
	default:
		return
	}
}
func ReachingErrorQMatMulTwo(q ReachingErrorQuant) (ok bool, err error) {
	err = errors.New("unsupported")
	switch q {
	case ReachingErrorA, ReachingErrorC:
		println("side effect")
		return ok, err
	default:
		return ok, err
	}
}
func ReachingErrorQMatMulThree(q ReachingErrorQuant) (ok bool, err error) {
	err = errors.New("unsupported")
	switch 1 {
	case 2:
		err = nil
	}
	switch q {
	case ReachingErrorA, ReachingErrorC:
		return
	default:
		return
	}
}
func ReachingErrorQMatMulFour(q ReachingErrorQuant) (ok bool, err error) {
	err = errors.New("unsupported")
	for range [0]int{} {
		err = nil
	}
	switch q {
	case ReachingErrorA, ReachingErrorC:
		return
	default:
		return
	}
}

// A boolean predicate from a rejected multi-result return must not manufacture
// positive return-guard coverage.
type DominantGuardErrorQuant uint8

const (
	DominantGuardErrorA DominantGuardErrorQuant = iota // want `quant variant DominantGuardErrorA \(DominantGuardErrorQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	DominantGuardErrorB                                // want `quant variant DominantGuardErrorB \(DominantGuardErrorQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	DominantGuardErrorC                                // want `quant variant DominantGuardErrorC \(DominantGuardErrorQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
)

func dominantGuardErrorByteSize(q DominantGuardErrorQuant) int {
	switch q {
	case DominantGuardErrorA, DominantGuardErrorB, DominantGuardErrorC:
		return 8
	default:
		return 0
	}
}
func dominantGuardErrorDecode(q DominantGuardErrorQuant) []float32 {
	switch q {
	case DominantGuardErrorA, DominantGuardErrorB, DominantGuardErrorC:
		return []float32{}
	default:
		return nil
	}
}
func DominantGuardErrorQMatMulOne(q DominantGuardErrorQuant) (bool, error) {
	return q == DominantGuardErrorA || q == DominantGuardErrorC, errors.New("unsupported")
}
func DominantGuardErrorQMatMulTwo(q DominantGuardErrorQuant) (bool, error) {
	return q == DominantGuardErrorA || q == DominantGuardErrorC, errors.New("unsupported")
}

// A predicate-bearing default supports only the values admitted by that
// predicate; it does not make the dispatch layer open-ended.
type DefaultPredicateQuant uint8

const (
	DefaultPredicateA DefaultPredicateQuant = iota
	DefaultPredicateB
	DefaultPredicateC // want `quant variant DefaultPredicateC \(DefaultPredicateQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
)

func defaultPredicateByteSize(q DefaultPredicateQuant) int {
	switch q {
	case DefaultPredicateA, DefaultPredicateB, DefaultPredicateC:
		return 8
	default:
		return 0
	}
}
func defaultPredicateDecode(q DefaultPredicateQuant) []float32 {
	switch q {
	case DefaultPredicateA, DefaultPredicateB, DefaultPredicateC:
		return []float32{}
	default:
		return nil
	}
}
func DefaultPredicateQMatMulOne(q DefaultPredicateQuant) bool {
	switch q {
	case DefaultPredicateA:
		return true
	default:
		return q == DefaultPredicateB
	}
}
func DefaultPredicateQMatMulTwo(q DefaultPredicateQuant) bool {
	switch q {
	case DefaultPredicateA:
		return true
	default:
		return q != DefaultPredicateC
	}
}

// Local aliases of explicit standard-library error constructors retain their
// proven rejection meaning without relying on the alias name.
type LocalErrorAliasQuant uint8

const (
	LocalErrorAliasA LocalErrorAliasQuant = iota
	LocalErrorAliasB                      // want `quant variant LocalErrorAliasB \(LocalErrorAliasQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	LocalErrorAliasC
)

func localErrorAliasByteSize(q LocalErrorAliasQuant) int {
	switch q {
	case LocalErrorAliasA, LocalErrorAliasB, LocalErrorAliasC:
		return 8
	default:
		return 0
	}
}
func localErrorAliasDecode(q LocalErrorAliasQuant) []float32 {
	switch q {
	case LocalErrorAliasA, LocalErrorAliasB, LocalErrorAliasC:
		return []float32{}
	default:
		return nil
	}
}
func LocalErrorAliasQMatMulOne(q LocalErrorAliasQuant) error {
	switch q {
	case LocalErrorAliasA, LocalErrorAliasC:
		return nil
	default:
		failure := errors.New("unsupported")
		return failure
	}
}
func LocalErrorAliasQMatMulTwo(q LocalErrorAliasQuant) error {
	switch q {
	case LocalErrorAliasA, LocalErrorAliasC:
		return nil
	default:
		rejected := errors.New("unsupported")
		return rejected
	}
}

// Explicit fallthrough clauses inherit the terminal target's result predicate.
type FallthroughPredicateQuant uint8

const (
	FallthroughPredicateA FallthroughPredicateQuant = iota
	FallthroughPredicateB                           // want `quant variant FallthroughPredicateB \(FallthroughPredicateQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	FallthroughPredicateC
)

func fallthroughPredicateByteSize(q FallthroughPredicateQuant) int {
	switch q {
	case FallthroughPredicateA, FallthroughPredicateB, FallthroughPredicateC:
		return 8
	default:
		return 0
	}
}
func fallthroughPredicateDecode(q FallthroughPredicateQuant) []float32 {
	switch q {
	case FallthroughPredicateA, FallthroughPredicateB, FallthroughPredicateC:
		return []float32{}
	default:
		return nil
	}
}
func FallthroughPredicateQMatMulOne(q FallthroughPredicateQuant) bool {
	switch q {
	case FallthroughPredicateA, FallthroughPredicateB:
		fallthrough
	case FallthroughPredicateC:
		return q == FallthroughPredicateA || q == FallthroughPredicateC
	default:
		return false
	}
}
func FallthroughPredicateQMatMulTwo(q FallthroughPredicateQuant) bool {
	switch q {
	case FallthroughPredicateA, FallthroughPredicateB:
		fallthrough
	case FallthroughPredicateC:
		return q != FallthroughPredicateB
	default:
		return false
	}
}

// A break can continue to an independently successful exit, so the terminal
// predicate alone must not narrow the case.
type BreakContinuationQuant uint8

const (
	BreakContinuationA BreakContinuationQuant = iota
	BreakContinuationB
	BreakContinuationC
)

func breakContinuationByteSize(q BreakContinuationQuant) int {
	switch q {
	case BreakContinuationA, BreakContinuationB, BreakContinuationC:
		return 8
	default:
		return 0
	}
}
func breakContinuationDecode(q BreakContinuationQuant) []float32 {
	switch q {
	case BreakContinuationA, BreakContinuationB, BreakContinuationC:
		return []float32{}
	default:
		return nil
	}
}
func BreakContinuationQMatMulOne(q BreakContinuationQuant, skip bool) bool {
	switch q {
	case BreakContinuationA, BreakContinuationB:
		if skip {
			break
		}
		return q == BreakContinuationA
	case BreakContinuationC:
		return true
	default:
		return false
	}
	return true
}
func BreakContinuationQMatMulTwo(q BreakContinuationQuant, skip bool) bool {
	switch q {
	case BreakContinuationA, BreakContinuationB:
		if skip {
			break
		}
		return q != BreakContinuationB
	case BreakContinuationC:
		return true
	default:
		return false
	}
	return true
}

// Range assignment mutates a package error sentinel and invalidates its
// initializer as proof that a returned value is rejection.
type RangeErrorMutationQuant uint8

const (
	RangeErrorMutationA RangeErrorMutationQuant = iota
	RangeErrorMutationB
	RangeErrorMutationC
)

var rangeErrorMutationFailure = errors.New("unsupported")

func rangeErrorMutationByteSize(q RangeErrorMutationQuant) int {
	switch q {
	case RangeErrorMutationA, RangeErrorMutationB, RangeErrorMutationC:
		return 8
	default:
		return 0
	}
}
func rangeErrorMutationDecode(q RangeErrorMutationQuant) []float32 {
	switch q {
	case RangeErrorMutationA, RangeErrorMutationB, RangeErrorMutationC:
		return []float32{}
	default:
		return nil
	}
}
func RangeErrorMutationQMatMulOne(q RangeErrorMutationQuant) error {
	switch q {
	case RangeErrorMutationA, RangeErrorMutationB:
		for _, rangeErrorMutationFailure = range []error{nil} {
		}
		return rangeErrorMutationFailure
	case RangeErrorMutationC:
		return nil
	default:
		return errors.New("unsupported")
	}
}
func RangeErrorMutationQMatMulTwo(q RangeErrorMutationQuant) error {
	switch q {
	case RangeErrorMutationA, RangeErrorMutationB:
		for _, rangeErrorMutationFailure = range []error{nil} {
		}
		return rangeErrorMutationFailure
	case RangeErrorMutationC:
		return nil
	default:
		return errors.New("unsupported")
	}
}

// An unresolved multi-result assignment invalidates an earlier named-result
// predicate instead of letting stale state narrow the case.
type StaleNamedResultQuant uint8

const (
	StaleNamedResultA StaleNamedResultQuant = iota
	StaleNamedResultB
	StaleNamedResultC
)

func staleNamedResultByteSize(q StaleNamedResultQuant) int {
	switch q {
	case StaleNamedResultA, StaleNamedResultB, StaleNamedResultC:
		return 8
	default:
		return 0
	}
}
func staleNamedResultDecode(q StaleNamedResultQuant) []float32 {
	switch q {
	case StaleNamedResultA, StaleNamedResultB, StaleNamedResultC:
		return []float32{}
	default:
		return nil
	}
}
func staleNamedResultProbe(StaleNamedResultQuant) (int, bool) { return 0, true }
func StaleNamedResultQMatMulOne(q StaleNamedResultQuant) (ok bool) {
	switch q {
	case StaleNamedResultA, StaleNamedResultB:
		ok = q == StaleNamedResultA
		_, ok = staleNamedResultProbe(q)
		return
	case StaleNamedResultC:
		ok = true
		return
	default:
		return
	}
}
func StaleNamedResultQMatMulTwo(q StaleNamedResultQuant) (ok bool) {
	switch q {
	case StaleNamedResultA, StaleNamedResultB:
		ok = q != StaleNamedResultB
		_, ok = staleNamedResultProbe(q)
		return
	case StaleNamedResultC:
		ok = true
		return
	default:
		return
	}
}

// A default predicate cannot re-admit a value intercepted by an explicit
// rejecting case.
type DefaultReachabilityQuant uint8

const (
	DefaultReachabilityA DefaultReachabilityQuant = iota
	DefaultReachabilityB                          // want `quant variant DefaultReachabilityB \(DefaultReachabilityQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	DefaultReachabilityC
)

func defaultReachabilityByteSize(q DefaultReachabilityQuant) int {
	switch q {
	case DefaultReachabilityA, DefaultReachabilityB, DefaultReachabilityC:
		return 8
	default:
		return 0
	}
}
func defaultReachabilityDecode(q DefaultReachabilityQuant) []float32 {
	switch q {
	case DefaultReachabilityA, DefaultReachabilityB, DefaultReachabilityC:
		return []float32{}
	default:
		return nil
	}
}
func DefaultReachabilityQMatMulOne(q DefaultReachabilityQuant) bool {
	switch q {
	case DefaultReachabilityB:
		return false
	case DefaultReachabilityA:
		return true
	default:
		return q == DefaultReachabilityB || q == DefaultReachabilityC
	}
}
func DefaultReachabilityQMatMulTwo(q DefaultReachabilityQuant) bool {
	switch q {
	case DefaultReachabilityB:
		return false
	case DefaultReachabilityA:
		return true
	default:
		return q != DefaultReachabilityA
	}
}

// A source-clause return prevents target-only fallthrough narrowing.
type FallthroughSourceExitQuant uint8

const (
	FallthroughSourceExitA FallthroughSourceExitQuant = iota
	FallthroughSourceExitB
	FallthroughSourceExitC
)

func fallthroughSourceExitByteSize(q FallthroughSourceExitQuant) int {
	switch q {
	case FallthroughSourceExitA, FallthroughSourceExitB, FallthroughSourceExitC:
		return 8
	default:
		return 0
	}
}
func fallthroughSourceExitDecode(q FallthroughSourceExitQuant) []float32 {
	switch q {
	case FallthroughSourceExitA, FallthroughSourceExitB, FallthroughSourceExitC:
		return []float32{}
	default:
		return nil
	}
}
func FallthroughSourceExitQMatMulOne(q FallthroughSourceExitQuant, early bool) bool {
	switch q {
	case FallthroughSourceExitA, FallthroughSourceExitB:
		if early {
			return true
		}
		fallthrough
	case FallthroughSourceExitC:
		return q == FallthroughSourceExitA || q == FallthroughSourceExitC
	default:
		return false
	}
}
func FallthroughSourceExitQMatMulTwo(q FallthroughSourceExitQuant, early bool) bool {
	switch q {
	case FallthroughSourceExitA, FallthroughSourceExitB:
		if early {
			return true
		}
		fallthrough
	case FallthroughSourceExitC:
		return q != FallthroughSourceExitB
	default:
		return false
	}
}

// A pointer alias created before the switch invalidates named-result
// narrowing inside its clauses.
type NamedResultPointerQuant uint8

const (
	NamedResultPointerA NamedResultPointerQuant = iota
	NamedResultPointerB
	NamedResultPointerC
)

func namedResultPointerByteSize(q NamedResultPointerQuant) int {
	switch q {
	case NamedResultPointerA, NamedResultPointerB, NamedResultPointerC:
		return 8
	default:
		return 0
	}
}
func namedResultPointerDecode(q NamedResultPointerQuant) []float32 {
	switch q {
	case NamedResultPointerA, NamedResultPointerB, NamedResultPointerC:
		return []float32{}
	default:
		return nil
	}
}
func NamedResultPointerQMatMulOne(q NamedResultPointerQuant) (ok bool) {
	pointer := &ok
	switch q {
	case NamedResultPointerA, NamedResultPointerB:
		ok = q == NamedResultPointerA
		*pointer = true
		return
	case NamedResultPointerC:
		return true
	default:
		return false
	}
}
func NamedResultPointerQMatMulTwo(q NamedResultPointerQuant) (ok bool) {
	pointer := &ok
	switch q {
	case NamedResultPointerA, NamedResultPointerB:
		ok = q != NamedResultPointerB
		*pointer = true
		return
	case NamedResultPointerC:
		return true
	default:
		return false
	}
}

// Suppressing switch-owned return evidence does not discard a predicate for a
// different enum returned from that clause.
type CrossReturnOuterQuant uint8
type CrossReturnInnerQuant uint8

const CrossReturnOuterA CrossReturnOuterQuant = 1

const (
	CrossReturnInnerA CrossReturnInnerQuant = iota
	CrossReturnInnerB                       // want `quant variant CrossReturnInnerB \(CrossReturnInnerQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	CrossReturnInnerC
)

func crossReturnInnerByteSize(q CrossReturnInnerQuant) int {
	switch q {
	case CrossReturnInnerA, CrossReturnInnerB, CrossReturnInnerC:
		return 8
	default:
		return 0
	}
}
func crossReturnInnerDecode(q CrossReturnInnerQuant) []float32 {
	switch q {
	case CrossReturnInnerA, CrossReturnInnerB, CrossReturnInnerC:
		return []float32{}
	default:
		return nil
	}
}
func CrossReturnQMatMulOne(outer CrossReturnOuterQuant, inner CrossReturnInnerQuant) bool {
	switch outer {
	case CrossReturnOuterA:
		return inner == CrossReturnInnerA || inner == CrossReturnInnerC
	default:
		return false
	}
}
func CrossReturnQMatMulTwo(outer CrossReturnOuterQuant, inner CrossReturnInnerQuant) bool {
	switch outer {
	case CrossReturnOuterA:
		return inner != CrossReturnInnerB
	default:
		return false
	}
}

// An explicit return of a named result uses its straight-line constructor
// assignment as rejection evidence.
type NamedErrorAssignmentQuant uint8

const (
	NamedErrorAssignmentA NamedErrorAssignmentQuant = iota
	NamedErrorAssignmentB                           // want `quant variant NamedErrorAssignmentB \(NamedErrorAssignmentQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	NamedErrorAssignmentC
)

func namedErrorAssignmentByteSize(q NamedErrorAssignmentQuant) int {
	switch q {
	case NamedErrorAssignmentA, NamedErrorAssignmentB, NamedErrorAssignmentC:
		return 8
	default:
		return 0
	}
}
func namedErrorAssignmentDecode(q NamedErrorAssignmentQuant) []float32 {
	switch q {
	case NamedErrorAssignmentA, NamedErrorAssignmentB, NamedErrorAssignmentC:
		return []float32{}
	default:
		return nil
	}
}
func NamedErrorAssignmentQMatMulOne(q NamedErrorAssignmentQuant) (err error) {
	switch q {
	case NamedErrorAssignmentA, NamedErrorAssignmentC:
		return nil
	default:
		err = errors.New("unsupported")
		return err
	}
}
func NamedErrorAssignmentQMatMulTwo(q NamedErrorAssignmentQuant) (err error) {
	switch q {
	case NamedErrorAssignmentA, NamedErrorAssignmentC:
		return nil
	default:
		err = errors.New("unsupported")
		return err
	}
}

// A conditional tagless case is not an unconditional subtraction from the
// default domain for the enum it happens to mention.
type TaglessConditionalDefaultQuant uint8

const (
	TaglessConditionalDefaultA TaglessConditionalDefaultQuant = iota
	TaglessConditionalDefaultB
	TaglessConditionalDefaultC
)

func taglessConditionalDefaultByteSize(q TaglessConditionalDefaultQuant) int {
	switch q {
	case TaglessConditionalDefaultA, TaglessConditionalDefaultB, TaglessConditionalDefaultC:
		return 8
	default:
		return 0
	}
}
func taglessConditionalDefaultDecode(q TaglessConditionalDefaultQuant) []float32 {
	switch q {
	case TaglessConditionalDefaultA, TaglessConditionalDefaultB, TaglessConditionalDefaultC:
		return []float32{}
	default:
		return nil
	}
}
func taglessConditionalDefaultOne(q TaglessConditionalDefaultQuant, enabled bool) bool {
	switch {
	case enabled && q == TaglessConditionalDefaultB:
		return false
	case q == TaglessConditionalDefaultA:
		return true
	default:
		return q == TaglessConditionalDefaultB || q == TaglessConditionalDefaultC
	}
}
func taglessConditionalDefaultTwo(q TaglessConditionalDefaultQuant, enabled bool) bool {
	switch {
	case enabled && q == TaglessConditionalDefaultB:
		return false
	case q == TaglessConditionalDefaultA:
		return true
	default:
		return q == TaglessConditionalDefaultB || q == TaglessConditionalDefaultC
	}
}
func TaglessConditionalDefaultQMatMul(q TaglessConditionalDefaultQuant, enabled bool) bool {
	return taglessConditionalDefaultOne(q, enabled) && taglessConditionalDefaultTwo(q, enabled)
}

// A direct return predicate belongs to its own tagless clause. A different
// clause mentioning the same enum must not suppress it.
type ClauseOwnedReturnQuant uint8

const (
	ClauseOwnedReturnA ClauseOwnedReturnQuant = iota
	ClauseOwnedReturnB                        // want `quant variant ClauseOwnedReturnB \(ClauseOwnedReturnQuant\).*absent from 4 of 4 reachable CPU matmul dispatch layers`
	ClauseOwnedReturnC
)

func clauseOwnedReturnByteSize(q ClauseOwnedReturnQuant) int {
	switch q {
	case ClauseOwnedReturnA, ClauseOwnedReturnB, ClauseOwnedReturnC:
		return 8
	default:
		return 0
	}
}
func clauseOwnedReturnDecode(q ClauseOwnedReturnQuant) []float32 {
	switch q {
	case ClauseOwnedReturnA, ClauseOwnedReturnB, ClauseOwnedReturnC:
		return []float32{}
	default:
		return nil
	}
}
func clauseOwnedReturnOne(q ClauseOwnedReturnQuant, route bool) bool {
	switch {
	case route:
		return q == ClauseOwnedReturnA || q == ClauseOwnedReturnC
	case q == ClauseOwnedReturnA, q == ClauseOwnedReturnC:
		return true
	default:
		return false
	}
}
func clauseOwnedReturnTwo(q ClauseOwnedReturnQuant, route bool) bool {
	switch {
	case route:
		return q == ClauseOwnedReturnA || q == ClauseOwnedReturnC
	case q == ClauseOwnedReturnA, q == ClauseOwnedReturnC:
		return true
	default:
		return false
	}
}
func ClauseOwnedReturnQMatMul(q ClauseOwnedReturnQuant, route bool) bool {
	return clauseOwnedReturnOne(q, route) && clauseOwnedReturnTwo(q, route)
}

// A conditional reassignment makes an explicit named error result unknown;
// the earlier constructor is not the value on every path.
type ConditionalNamedErrorQuant uint8

const (
	ConditionalNamedErrorA ConditionalNamedErrorQuant = iota
	ConditionalNamedErrorB
	ConditionalNamedErrorC
)

func conditionalNamedErrorByteSize(q ConditionalNamedErrorQuant) int {
	switch q {
	case ConditionalNamedErrorA, ConditionalNamedErrorB, ConditionalNamedErrorC:
		return 8
	default:
		return 0
	}
}
func conditionalNamedErrorDecode(q ConditionalNamedErrorQuant) []float32 {
	switch q {
	case ConditionalNamedErrorA, ConditionalNamedErrorB, ConditionalNamedErrorC:
		return []float32{}
	default:
		return nil
	}
}
func ConditionalNamedErrorQMatMulOne(q ConditionalNamedErrorQuant, recoverError bool) (err error) {
	switch q {
	case ConditionalNamedErrorA, ConditionalNamedErrorB:
		err = errors.New("unsupported")
		if recoverError {
			err = nil
		}
		return err
	default:
		return nil
	}
}
func ConditionalNamedErrorQMatMulTwo(q ConditionalNamedErrorQuant, recoverError bool) (err error) {
	switch q {
	case ConditionalNamedErrorA, ConditionalNamedErrorB:
		err = errors.New("unsupported")
		if recoverError {
			err = nil
		}
		return err
	default:
		return nil
	}
}

// Conditional writes also make a naked named result path-dependent rather
// than preserving the last top-level assignment.
type ConditionalNamedBoolQuant uint8

const (
	ConditionalNamedBoolA ConditionalNamedBoolQuant = iota
	ConditionalNamedBoolB
	ConditionalNamedBoolC
)

func conditionalNamedBoolByteSize(q ConditionalNamedBoolQuant) int {
	switch q {
	case ConditionalNamedBoolA, ConditionalNamedBoolB, ConditionalNamedBoolC:
		return 8
	default:
		return 0
	}
}
func conditionalNamedBoolDecode(q ConditionalNamedBoolQuant) []float32 {
	switch q {
	case ConditionalNamedBoolA, ConditionalNamedBoolB, ConditionalNamedBoolC:
		return []float32{}
	default:
		return nil
	}
}
func ConditionalNamedBoolQMatMulOne(q ConditionalNamedBoolQuant, enabled bool) (ok bool) {
	switch q {
	case ConditionalNamedBoolA, ConditionalNamedBoolB:
		ok = false
		if enabled {
			ok = true
		}
		return
	default:
		ok = true
		return
	}
}
func ConditionalNamedBoolQMatMulTwo(q ConditionalNamedBoolQuant, enabled bool) (ok bool) {
	switch q {
	case ConditionalNamedBoolA, ConditionalNamedBoolB:
		ok = false
		if enabled {
			ok = true
		}
		return
	default:
		ok = true
		return
	}
}

// Predicate narrowing keeps same-typed enum variables distinct from the
// switch subject.
type SubjectIdentityQuant uint8

const (
	SubjectIdentityA SubjectIdentityQuant = iota // want `quant variant SubjectIdentityA \(SubjectIdentityQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	SubjectIdentityB                             // want `quant variant SubjectIdentityB \(SubjectIdentityQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	SubjectIdentityC                             // want `quant variant SubjectIdentityC \(SubjectIdentityQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
)

func subjectIdentityByteSize(q SubjectIdentityQuant) int {
	switch q {
	case SubjectIdentityA, SubjectIdentityB, SubjectIdentityC:
		return 8
	default:
		return 0
	}
}
func subjectIdentityDecode(q SubjectIdentityQuant) []float32 {
	switch q {
	case SubjectIdentityA, SubjectIdentityB, SubjectIdentityC:
		return []float32{}
	default:
		return nil
	}
}
func SubjectIdentityQMatMulOne(q, other SubjectIdentityQuant) bool {
	switch q {
	case SubjectIdentityA:
		return false
	default:
		return other == SubjectIdentityA
	}
}
func SubjectIdentityQMatMulTwo(q, other SubjectIdentityQuant) bool {
	switch q {
	case SubjectIdentityA:
		return false
	default:
		return other == SubjectIdentityA
	}
}

// A shape disjunction can enter the clause without satisfying its enum arm,
// so its independent return predicate remains a separate layer.
type MixedShapeOrQuant uint8

const (
	MixedShapeOrA MixedShapeOrQuant = iota // want `quant variant MixedShapeOrA \(MixedShapeOrQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	MixedShapeOrB                          // want `quant variant MixedShapeOrB \(MixedShapeOrQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	MixedShapeOrC
)

func mixedShapeOrByteSize(q MixedShapeOrQuant) int {
	switch q {
	case MixedShapeOrA, MixedShapeOrB, MixedShapeOrC:
		return 8
	default:
		return 0
	}
}
func mixedShapeOrDecode(q MixedShapeOrQuant) []float32 {
	switch q {
	case MixedShapeOrA, MixedShapeOrB, MixedShapeOrC:
		return []float32{}
	default:
		return nil
	}
}
func mixedShapeOrOne(q MixedShapeOrQuant, enabled bool) bool {
	switch {
	case enabled || q == MixedShapeOrC:
		return q == MixedShapeOrA || q == MixedShapeOrB || q == MixedShapeOrC
	default:
		return false
	}
}
func mixedShapeOrTwo(q MixedShapeOrQuant, enabled bool) bool {
	switch {
	case enabled || q == MixedShapeOrC:
		return q == MixedShapeOrA || q == MixedShapeOrB || q == MixedShapeOrC
	default:
		return false
	}
}
func MixedShapeOrQMatMul(q MixedShapeOrQuant, enabled bool) bool {
	return mixedShapeOrOne(q, enabled) && mixedShapeOrTwo(q, enabled)
}

// An unassigned named error result has its implicit nil value at a bare
// return and therefore denotes success.
type ImplicitNilErrorQuant uint8

const (
	ImplicitNilErrorA ImplicitNilErrorQuant = iota
	ImplicitNilErrorB
	ImplicitNilErrorC
)

func implicitNilErrorByteSize(q ImplicitNilErrorQuant) int {
	switch q {
	case ImplicitNilErrorA, ImplicitNilErrorB, ImplicitNilErrorC:
		return 8
	default:
		return 0
	}
}
func implicitNilErrorDecode(q ImplicitNilErrorQuant) []float32 {
	switch q {
	case ImplicitNilErrorA, ImplicitNilErrorB, ImplicitNilErrorC:
		return []float32{}
	default:
		return nil
	}
}
func ImplicitNilErrorQMatMulOne(q ImplicitNilErrorQuant) (err error) {
	switch q {
	case ImplicitNilErrorA, ImplicitNilErrorB:
		return
	case ImplicitNilErrorC:
		return nil
	default:
		return errors.New("unsupported")
	}
}
func ImplicitNilErrorQMatMulTwo(q ImplicitNilErrorQuant) (err error) {
	switch q {
	case ImplicitNilErrorA, ImplicitNilErrorB:
		return
	case ImplicitNilErrorC:
		return nil
	default:
		return errors.New("unsupported")
	}
}

// Merely containing an enum variable does not prove that a transformed
// switch subject has the same values as that variable.
type TransformedSubjectQuant uint8

const (
	TransformedSubjectA TransformedSubjectQuant = iota // want `quant variant TransformedSubjectA \(TransformedSubjectQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	TransformedSubjectB                                // want `quant variant TransformedSubjectB \(TransformedSubjectQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	TransformedSubjectC                                // want `quant variant TransformedSubjectC \(TransformedSubjectQuant\).*absent from 4 of 4 reachable CPU matmul dispatch layers`
)

func transformedSubjectByteSize(q TransformedSubjectQuant) int {
	switch q {
	case TransformedSubjectA, TransformedSubjectB, TransformedSubjectC:
		return 8
	default:
		return 0
	}
}
func transformedSubjectDecode(q TransformedSubjectQuant) []float32 {
	switch q {
	case TransformedSubjectA, TransformedSubjectB, TransformedSubjectC:
		return []float32{}
	default:
		return nil
	}
}
func TransformedSubjectQMatMulOne(q TransformedSubjectQuant) bool {
	switch q + 1 {
	case TransformedSubjectB:
		return q == TransformedSubjectA
	default:
		return false
	}
}
func TransformedSubjectQMatMulTwo(q TransformedSubjectQuant) bool {
	switch {
	case q+1 == TransformedSubjectB:
		return q == TransformedSubjectA
	default:
		return false
	}
}

// Every named result of a bare return is evaluated independently: known
// assignments are retained and untouched results keep their implicit zero.
type PartialNamedResultsQuant uint8

const (
	PartialNamedResultsA PartialNamedResultsQuant = iota
	PartialNamedResultsB
	PartialNamedResultsC
)

func partialNamedResultsByteSize(q PartialNamedResultsQuant) int {
	switch q {
	case PartialNamedResultsA, PartialNamedResultsB, PartialNamedResultsC:
		return 8
	default:
		return 0
	}
}
func partialNamedResultsDecode(q PartialNamedResultsQuant) []float32 {
	switch q {
	case PartialNamedResultsA, PartialNamedResultsB, PartialNamedResultsC:
		return []float32{}
	default:
		return nil
	}
}
func PartialNamedResultsQMatMulOne(q PartialNamedResultsQuant) (ok bool, err error) {
	switch q {
	case PartialNamedResultsA, PartialNamedResultsB:
		err = nil
		return
	case PartialNamedResultsC:
		return true, nil
	default:
		return false, errors.New("unsupported")
	}
}
func PartialNamedResultsQMatMulTwo(q PartialNamedResultsQuant) (ok bool, err error) {
	switch q {
	case PartialNamedResultsA, PartialNamedResultsB:
		err = nil
		return
	case PartialNamedResultsC:
		return true, nil
	default:
		return false, errors.New("unsupported")
	}
}

// Every enum-bearing arm must preserve value identity; a direct sibling arm
// cannot make a transformed arm safe for default subtraction or filtering.
type SiblingTransformQuant uint8

const (
	SiblingTransformA SiblingTransformQuant = iota // want `quant variant SiblingTransformA \(SiblingTransformQuant\).*absent from 4 of 4 reachable CPU matmul dispatch layers`
	SiblingTransformB                              // want `quant variant SiblingTransformB \(SiblingTransformQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	SiblingTransformC                              // want `quant variant SiblingTransformC \(SiblingTransformQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
)

func siblingTransformByteSize(q SiblingTransformQuant) int {
	switch q {
	case SiblingTransformA, SiblingTransformB, SiblingTransformC:
		return 8
	default:
		return 0
	}
}
func siblingTransformDecode(q SiblingTransformQuant) []float32 {
	switch q {
	case SiblingTransformA, SiblingTransformB, SiblingTransformC:
		return []float32{}
	default:
		return nil
	}
}
func siblingTransformOne(q SiblingTransformQuant) bool {
	switch {
	case q == SiblingTransformA || q+1 == SiblingTransformC:
		return false
	default:
		return q == SiblingTransformC
	}
}
func siblingTransformTwo(q SiblingTransformQuant) bool {
	switch {
	case q == SiblingTransformA || q+1 == SiblingTransformC:
		return false
	default:
		return q == SiblingTransformC
	}
}
func SiblingTransformQMatMul(q SiblingTransformQuant) bool {
	return siblingTransformOne(q) && siblingTransformTwo(q)
}

// Compound assignments update rather than replace a named result, so their
// reaching value is kept unknown unless the operation itself is evaluated.
type CompoundNamedResultQuant uint8

const (
	CompoundNamedResultA CompoundNamedResultQuant = iota
	CompoundNamedResultB
	CompoundNamedResultC
)

func compoundNamedResultByteSize(q CompoundNamedResultQuant) int {
	switch q {
	case CompoundNamedResultA, CompoundNamedResultB, CompoundNamedResultC:
		return 8
	default:
		return 0
	}
}
func compoundNamedResultDecode(q CompoundNamedResultQuant) []float32 {
	switch q {
	case CompoundNamedResultA, CompoundNamedResultB, CompoundNamedResultC:
		return []float32{}
	default:
		return nil
	}
}
func CompoundNamedResultQMatMulOne(q CompoundNamedResultQuant) (n int) {
	switch q {
	case CompoundNamedResultA, CompoundNamedResultB:
		n = 1
		n += 0
		return
	case CompoundNamedResultC:
		return 1
	default:
		return 0
	}
}
func CompoundNamedResultQMatMulTwo(q CompoundNamedResultQuant) (n int) {
	switch q {
	case CompoundNamedResultA, CompoundNamedResultB:
		n = 1
		n += 0
		return n
	case CompoundNamedResultC:
		return 1
	default:
		return 0
	}
}

// Predicate returns in else and else-if branches are mutually exclusive
// alternatives of the guarding if, not independent dispatch layers.
type ElseReturnQuant uint8

const (
	ElseReturnA ElseReturnQuant = iota
	ElseReturnB
	ElseReturnC
)

func elseReturnByteSize(q ElseReturnQuant) int {
	switch q {
	case ElseReturnA, ElseReturnB, ElseReturnC:
		return 8
	default:
		return 0
	}
}
func elseReturnDecode(q ElseReturnQuant) []float32 {
	switch q {
	case ElseReturnA, ElseReturnB, ElseReturnC:
		return []float32{}
	default:
		return nil
	}
}
func ElseReturnQMatMulOne(q ElseReturnQuant) bool {
	if q == ElseReturnA {
		return true
	} else {
		return q == ElseReturnB || q == ElseReturnC
	}
}
func ElseReturnQMatMulTwo(q ElseReturnQuant) bool {
	if q == ElseReturnA {
		return true
	} else if q == ElseReturnB {
		return true
	} else {
		return q == ElseReturnC
	}
}

// Alternative groups use boolean union: support on one mutually exclusive
// route removes rejection evidence from another route.
type AlternativeUnionQuant uint8

const (
	AlternativeUnionA AlternativeUnionQuant = iota
	AlternativeUnionB
	AlternativeUnionC
)

func alternativeUnionByteSize(q AlternativeUnionQuant) int {
	switch q {
	case AlternativeUnionA, AlternativeUnionB, AlternativeUnionC:
		return 8
	default:
		return 0
	}
}
func alternativeUnionDecode(q AlternativeUnionQuant) []float32 {
	switch q {
	case AlternativeUnionA, AlternativeUnionB, AlternativeUnionC:
		return []float32{}
	default:
		return nil
	}
}
func AlternativeUnionQMatMulOne(q AlternativeUnionQuant, enabled bool) bool {
	if enabled && q == AlternativeUnionA {
		return false
	} else {
		return q == AlternativeUnionA || q == AlternativeUnionB || q == AlternativeUnionC
	}
}
func AlternativeUnionQMatMulTwo(q AlternativeUnionQuant, enabled bool) bool {
	if enabled && q == AlternativeUnionA {
		return false
	} else {
		return q == AlternativeUnionA || q == AlternativeUnionB || q == AlternativeUnionC
	}
}

// A shape-only outer branch can route to exactly one enum dispatch site in
// each arm; those descendants form one position-scoped alternative layer.
type NestedShapeAlternativesQuant uint8

const (
	NestedShapeAlternativesA NestedShapeAlternativesQuant = iota
	NestedShapeAlternativesB
	NestedShapeAlternativesC
)

func nestedShapeAlternativesByteSize(q NestedShapeAlternativesQuant) int {
	switch q {
	case NestedShapeAlternativesA, NestedShapeAlternativesB, NestedShapeAlternativesC:
		return 8
	default:
		return 0
	}
}
func nestedShapeAlternativesDecode(q NestedShapeAlternativesQuant) []float32 {
	switch q {
	case NestedShapeAlternativesA, NestedShapeAlternativesB, NestedShapeAlternativesC:
		return []float32{}
	default:
		return nil
	}
}
func NestedShapeAlternativesQMatMulOne(q NestedShapeAlternativesQuant, enabled bool) bool {
	if enabled {
		if q == NestedShapeAlternativesA || q == NestedShapeAlternativesC {
			return true
		}
	} else {
		return q == NestedShapeAlternativesB
	}
	return false
}
func NestedShapeAlternativesQMatMulTwo(q NestedShapeAlternativesQuant, enabled bool) bool {
	if enabled {
		if q == NestedShapeAlternativesA || q == NestedShapeAlternativesC {
			return true
		}
	} else {
		return q == NestedShapeAlternativesB
	}
	return false
}

// Position-scoped shape alternatives compose transitively through nested
// if/else routing when each arm contributes one logical dispatch group.
type TransitiveShapeAlternativesQuant uint8

const (
	TransitiveShapeAlternativesA TransitiveShapeAlternativesQuant = iota
	TransitiveShapeAlternativesB
	TransitiveShapeAlternativesC
)

func transitiveShapeAlternativesByteSize(q TransitiveShapeAlternativesQuant) int {
	switch q {
	case TransitiveShapeAlternativesA, TransitiveShapeAlternativesB, TransitiveShapeAlternativesC:
		return 8
	default:
		return 0
	}
}
func transitiveShapeAlternativesDecode(q TransitiveShapeAlternativesQuant) []float32 {
	switch q {
	case TransitiveShapeAlternativesA, TransitiveShapeAlternativesB, TransitiveShapeAlternativesC:
		return []float32{}
	default:
		return nil
	}
}
func TransitiveShapeAlternativesQMatMulOne(q TransitiveShapeAlternativesQuant, enabled, route bool) bool {
	if enabled {
		if route {
			return q == TransitiveShapeAlternativesA
		} else {
			return q == TransitiveShapeAlternativesC
		}
	} else {
		return q == TransitiveShapeAlternativesB
	}
}
func TransitiveShapeAlternativesQMatMulTwo(q TransitiveShapeAlternativesQuant, enabled, route bool) bool {
	if enabled {
		if route {
			return q == TransitiveShapeAlternativesA
		} else {
			return q == TransitiveShapeAlternativesC
		}
	} else {
		return q == TransitiveShapeAlternativesB
	}
}

// Lexical range containment never groups a return inside a function literal
// with a branch in the enclosing function.
type LiteralShapeBoundaryQuant uint8

const (
	LiteralShapeBoundaryA LiteralShapeBoundaryQuant = iota // want `quant variant LiteralShapeBoundaryA \(LiteralShapeBoundaryQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	LiteralShapeBoundaryB                                  // want `quant variant LiteralShapeBoundaryB \(LiteralShapeBoundaryQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	LiteralShapeBoundaryC                                  // want `quant variant LiteralShapeBoundaryC \(LiteralShapeBoundaryQuant\).*absent from 4 of 4 reachable CPU matmul dispatch layers`
)

func literalShapeBoundaryByteSize(q LiteralShapeBoundaryQuant) int {
	switch q {
	case LiteralShapeBoundaryA, LiteralShapeBoundaryB, LiteralShapeBoundaryC:
		return 8
	default:
		return 0
	}
}
func literalShapeBoundaryDecode(q LiteralShapeBoundaryQuant) []float32 {
	switch q {
	case LiteralShapeBoundaryA, LiteralShapeBoundaryB, LiteralShapeBoundaryC:
		return []float32{}
	default:
		return nil
	}
}
func LiteralShapeBoundaryQMatMulOne(q LiteralShapeBoundaryQuant, enabled bool) bool {
	if enabled {
		dispatch := func() bool { return q == LiteralShapeBoundaryA }
		_ = dispatch()
		return false
	} else {
		return q == LiteralShapeBoundaryB
	}
}
func LiteralShapeBoundaryQMatMulTwo(q LiteralShapeBoundaryQuant, enabled bool) bool {
	if enabled {
		dispatch := func() bool { return q == LiteralShapeBoundaryA }
		_ = dispatch()
		return false
	} else {
		return q == LiteralShapeBoundaryB
	}
}

// Shape alternatives apply across site kinds, including a switch in each
// arm, while remaining one logical dispatch layer per function.
type ShapeSwitchAlternativesQuant uint8

const (
	ShapeSwitchAlternativesA ShapeSwitchAlternativesQuant = iota
	ShapeSwitchAlternativesB
	ShapeSwitchAlternativesC
)

func shapeSwitchAlternativesByteSize(q ShapeSwitchAlternativesQuant) int {
	switch q {
	case ShapeSwitchAlternativesA, ShapeSwitchAlternativesB, ShapeSwitchAlternativesC:
		return 8
	default:
		return 0
	}
}
func shapeSwitchAlternativesDecode(q ShapeSwitchAlternativesQuant) []float32 {
	switch q {
	case ShapeSwitchAlternativesA, ShapeSwitchAlternativesB, ShapeSwitchAlternativesC:
		return []float32{}
	default:
		return nil
	}
}
func shapeSwitchAlternativesOne(q ShapeSwitchAlternativesQuant, enabled bool) bool {
	if enabled {
		switch q {
		case ShapeSwitchAlternativesA, ShapeSwitchAlternativesC:
			return true
		default:
			return false
		}
	} else {
		switch q {
		case ShapeSwitchAlternativesB:
			return true
		default:
			return false
		}
	}
}
func shapeSwitchAlternativesTwo(q ShapeSwitchAlternativesQuant, enabled bool) bool {
	if enabled {
		switch q {
		case ShapeSwitchAlternativesA, ShapeSwitchAlternativesC:
			return true
		default:
			return false
		}
	} else {
		switch q {
		case ShapeSwitchAlternativesB:
			return true
		default:
			return false
		}
	}
}
func ShapeSwitchAlternativesQMatMul(q ShapeSwitchAlternativesQuant, enabled bool) bool {
	return shapeSwitchAlternativesOne(q, enabled) && shapeSwitchAlternativesTwo(q, enabled)
}

// Transitive map aliases can change a package dispatch table and therefore
// invalidate initializer-based coverage.
type AliasMutationQuant uint8

const (
	AliasMutationA AliasMutationQuant = iota
	AliasMutationB                    // want `quant variant AliasMutationB \(AliasMutationQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	AliasMutationC
)

func aliasMutationByteSize(q AliasMutationQuant) int {
	switch q {
	case AliasMutationA, AliasMutationB, AliasMutationC:
		return 8
	default:
		return 0
	}
}
func aliasMutationDecode(q AliasMutationQuant) []float32 {
	switch q {
	case AliasMutationA, AliasMutationB, AliasMutationC:
		return []float32{}
	default:
		return nil
	}
}

var aliasMutationDispatch = map[AliasMutationQuant]func(){
	AliasMutationA: func() {}, AliasMutationC: func() {},
}
var aliasEscapeDispatch = map[AliasMutationQuant]func(){
	AliasMutationA: func() {}, AliasMutationC: func() {},
}
var aliasEscapeView = aliasEscapeDispatch
var aggregateMutationDispatch = map[AliasMutationQuant]func(){
	AliasMutationA: func() {}, AliasMutationC: func() {},
}
var staleAliasDispatch = map[AliasMutationQuant]func(){
	AliasMutationA: func() {}, AliasMutationC: func() {},
}
var switchAliasDispatch = map[AliasMutationQuant]func(){
	AliasMutationA: func() {}, AliasMutationC: func() {},
}

func completeAliasMutationDispatch() {
	first := aliasMutationDispatch
	second := first
	second[AliasMutationB] = func() {}
}
func addAliasMutationEntry(table map[AliasMutationQuant]func()) {
	table[AliasMutationB] = func() {}
}
func completeAliasEscapeDispatch() {
	second := aliasEscapeView
	addAliasMutationEntry(second)
}
func completeAggregateMutationDispatch() {
	holder := struct {
		table map[AliasMutationQuant]func()
	}{table: aggregateMutationDispatch}
	holder.table[AliasMutationB] = func() {}
}
func mutateFreshAliasMap() {
	view := staleAliasDispatch
	view = make(map[AliasMutationQuant]func())
	view[AliasMutationB] = func() {}
}
func completeSwitchAliasDispatch(q AliasMutationQuant, route int) bool {
	view := switchAliasDispatch
	switch route {
	case 1:
		view = make(map[AliasMutationQuant]func())
		return aliasMutationAll(q)
	case 2:
		view[AliasMutationB] = func() {}
		_, ok := switchAliasDispatch[q]
		return ok && aliasMutationAll(q)
	default:
		return false
	}
}
func aliasMutationAll(q AliasMutationQuant) bool {
	return q == AliasMutationA || q == AliasMutationB || q == AliasMutationC
}
func AliasMutationQMatMul(q AliasMutationQuant, route int) bool {
	completeAliasMutationDispatch()
	completeAliasEscapeDispatch()
	completeAggregateMutationDispatch()
	mutateFreshAliasMap()
	_, directOK := aliasMutationDispatch[q]
	_, escapedOK := aliasEscapeDispatch[q]
	_, aggregateOK := aggregateMutationDispatch[q]
	_, staleOK := staleAliasDispatch[q]
	return directOK && escapedOK && aggregateOK && staleOK &&
		completeSwitchAliasDispatch(q, route) && aliasMutationAll(q)
}

// Rebinding a function variable in one switch case does not affect a call in
// a mutually exclusive case.
type CaseLiteralQuant uint8

const (
	CaseLiteralA CaseLiteralQuant = iota
	CaseLiteralB                  // want `quant variant CaseLiteralB \(CaseLiteralQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	CaseLiteralC
)

func caseLiteralByteSize(q CaseLiteralQuant) int {
	switch q {
	case CaseLiteralA, CaseLiteralB, CaseLiteralC:
		return 8
	default:
		return 0
	}
}
func caseLiteralDecode(q CaseLiteralQuant) []float32 {
	switch q {
	case CaseLiteralA, CaseLiteralB, CaseLiteralC:
		return []float32{}
	default:
		return nil
	}
}
func caseLiteralNamed(q CaseLiteralQuant) bool {
	return q == CaseLiteralA || q == CaseLiteralC
}
func CaseLiteralQMatMul(q CaseLiteralQuant, route int) bool {
	dispatch := func() bool { return q == CaseLiteralA || q == CaseLiteralC }
	switch route {
	case 1:
		dispatch = func() bool { return true }
		return false
	case 2:
		return dispatch() && caseLiteralNamed(q)
	default:
		return false
	}
}

// Stable global dispatch tables remain visible when indexed through a proven
// local alias.
type GlobalAliasQuant uint8

const (
	GlobalAliasA GlobalAliasQuant = iota
	GlobalAliasB                  // want `quant variant GlobalAliasB \(GlobalAliasQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	GlobalAliasC
)

func globalAliasByteSize(q GlobalAliasQuant) int {
	switch q {
	case GlobalAliasA, GlobalAliasB, GlobalAliasC:
		return 8
	default:
		return 0
	}
}
func globalAliasDecode(q GlobalAliasQuant) []float32 {
	switch q {
	case GlobalAliasA, GlobalAliasB, GlobalAliasC:
		return []float32{}
	default:
		return nil
	}
}

var globalAliasDispatch = map[GlobalAliasQuant]func(){
	GlobalAliasA: func() {}, GlobalAliasC: func() {},
}

func globalAliasNamed(q GlobalAliasQuant) bool {
	return q == GlobalAliasA || q == GlobalAliasC
}
func GlobalAliasQMatMul(q GlobalAliasQuant) bool {
	view := globalAliasDispatch
	_, ok := view[q]
	return ok && globalAliasNamed(q)
}

// A parameter does not denote a global table merely because a later
// assignment makes it an alias candidate.
type FutureAliasQuant uint8

const (
	FutureAliasA FutureAliasQuant = iota
	FutureAliasB
	FutureAliasC
)

func futureAliasByteSize(q FutureAliasQuant) int {
	switch q {
	case FutureAliasA, FutureAliasB, FutureAliasC:
		return 8
	default:
		return 0
	}
}
func futureAliasDecode(q FutureAliasQuant) []float32 {
	switch q {
	case FutureAliasA, FutureAliasB, FutureAliasC:
		return []float32{}
	default:
		return nil
	}
}

var futureAliasDispatch = map[FutureAliasQuant]func(){
	FutureAliasA: func() {}, FutureAliasC: func() {},
}

func futureAliasNamed(q FutureAliasQuant) bool {
	return q == FutureAliasA || q == FutureAliasC
}
func FutureAliasQMatMul(q FutureAliasQuant, other map[FutureAliasQuant]func()) bool {
	before := other[q] != nil
	other = futureAliasDispatch
	{
		other = make(map[FutureAliasQuant]func())
	}
	after := other[q] != nil
	return before && after && futureAliasNamed(q)
}

// A dead assignment does not turn a package variable initialized to another
// map into an alias of the tracked partial table.
type DeadPackageAliasQuant uint8

const (
	DeadPackageAliasA DeadPackageAliasQuant = iota
	DeadPackageAliasB
	DeadPackageAliasC
)

func deadPackageAliasByteSize(q DeadPackageAliasQuant) int {
	switch q {
	case DeadPackageAliasA, DeadPackageAliasB, DeadPackageAliasC:
		return 8
	default:
		return 0
	}
}
func deadPackageAliasDecode(q DeadPackageAliasQuant) []float32 {
	switch q {
	case DeadPackageAliasA, DeadPackageAliasB, DeadPackageAliasC:
		return []float32{}
	default:
		return nil
	}
}

var deadPackageAliasPartial = map[DeadPackageAliasQuant]func(){
	DeadPackageAliasA: func() {}, DeadPackageAliasC: func() {},
}
var deadPackageAliasComplete = map[DeadPackageAliasQuant]func(){
	DeadPackageAliasA: func() {}, DeadPackageAliasB: func() {}, DeadPackageAliasC: func() {},
}

func deadPackageAliasAll(q DeadPackageAliasQuant) bool {
	return q == DeadPackageAliasA || q == DeadPackageAliasB || q == DeadPackageAliasC
}
func DeadPackageAliasQMatMul(q DeadPackageAliasQuant) bool {
	if false {
		deadPackageAliasComplete = deadPackageAliasPartial
	}
	_, ok := deadPackageAliasComplete[q]
	return ok && deadPackageAliasAll(q)
}

// Package-entry aliases reflect reachable init rebindings, not only variable
// declaration initializers.
type InitRebindAliasQuant uint8

const (
	InitRebindAliasA InitRebindAliasQuant = iota
	InitRebindAliasB
	InitRebindAliasC
)

func initRebindAliasByteSize(q InitRebindAliasQuant) int {
	switch q {
	case InitRebindAliasA, InitRebindAliasB, InitRebindAliasC:
		return 8
	default:
		return 0
	}
}
func initRebindAliasDecode(q InitRebindAliasQuant) []float32 {
	switch q {
	case InitRebindAliasA, InitRebindAliasB, InitRebindAliasC:
		return []float32{}
	default:
		return nil
	}
}

var initRebindAliasPartial = map[InitRebindAliasQuant]func(){
	InitRebindAliasA: func() {}, InitRebindAliasC: func() {},
}
var initRebindAliasView = initRebindAliasPartial
var convertedInitAliasPartial = map[InitRebindAliasQuant]func(){
	InitRebindAliasA: func() {}, InitRebindAliasC: func() {},
}
var convertedInitAliasView = convertedInitAliasPartial

type initAliasSetter func()

func setInitRebindAliasFull() {
	initRebindAliasView = map[InitRebindAliasQuant]func(){
		InitRebindAliasA: func() {}, InitRebindAliasB: func() {}, InitRebindAliasC: func() {},
	}
}
func runInitAliasSetter(setter func()) { setter() }
func setConvertedInitAliasFull() {
	convertedInitAliasView = map[InitRebindAliasQuant]func(){
		InitRebindAliasA: func() {}, InitRebindAliasB: func() {}, InitRebindAliasC: func() {},
	}
}
func getInitAliasSetter() func() {
	return func() {
		initRebindAliasView = map[InitRebindAliasQuant]func(){
			InitRebindAliasA: func() {}, InitRebindAliasB: func() {}, InitRebindAliasC: func() {},
		}
	}
}
func init() {
	runInitAliasSetter(getInitAliasSetter())
	runInitAliasSetter(initAliasSetter(setConvertedInitAliasFull))
}
func initRebindAliasAll(q InitRebindAliasQuant) bool {
	return q == InitRebindAliasA || q == InitRebindAliasB || q == InitRebindAliasC
}
func InitRebindAliasQMatMul(q InitRebindAliasQuant) bool {
	_, factoryOK := initRebindAliasView[q]
	_, convertedOK := convertedInitAliasView[q]
	return factoryOK && convertedOK && initRebindAliasAll(q)
}

// Init can also establish the package alias used by a dispatch lookup.
type InitEstablishedAliasQuant uint8

const (
	InitEstablishedAliasA InitEstablishedAliasQuant = iota
	InitEstablishedAliasB                           // want `quant variant InitEstablishedAliasB \(InitEstablishedAliasQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	InitEstablishedAliasC
)

func initEstablishedAliasByteSize(q InitEstablishedAliasQuant) int {
	switch q {
	case InitEstablishedAliasA, InitEstablishedAliasB, InitEstablishedAliasC:
		return 8
	default:
		return 0
	}
}
func initEstablishedAliasDecode(q InitEstablishedAliasQuant) []float32 {
	switch q {
	case InitEstablishedAliasA, InitEstablishedAliasB, InitEstablishedAliasC:
		return []float32{}
	default:
		return nil
	}
}

var initEstablishedAliasPartial = map[InitEstablishedAliasQuant]func(){
	InitEstablishedAliasA: func() {}, InitEstablishedAliasC: func() {},
}
var initEstablishedAliasView map[InitEstablishedAliasQuant]func()

func establishInitAlias() {
	initEstablishedAliasView = initEstablishedAliasPartial
}
func init() { establishInitAlias() }
func initEstablishedAliasNamed(q InitEstablishedAliasQuant) bool {
	return q == InitEstablishedAliasA || q == InitEstablishedAliasC
}
func InitEstablishedAliasQMatMul(q InitEstablishedAliasQuant) bool {
	_, ok := initEstablishedAliasView[q]
	return ok && initEstablishedAliasNamed(q)
}

// A conditional init assignment yields unknown entry state rather than a
// source-order alias guess.
type ConditionalInitAliasQuant uint8

const (
	ConditionalInitAliasA ConditionalInitAliasQuant = iota
	ConditionalInitAliasB
	ConditionalInitAliasC
)

func conditionalInitAliasByteSize(q ConditionalInitAliasQuant) int {
	switch q {
	case ConditionalInitAliasA, ConditionalInitAliasB, ConditionalInitAliasC:
		return 8
	default:
		return 0
	}
}
func conditionalInitAliasDecode(q ConditionalInitAliasQuant) []float32 {
	switch q {
	case ConditionalInitAliasA, ConditionalInitAliasB, ConditionalInitAliasC:
		return []float32{}
	default:
		return nil
	}
}

var conditionalInitAliasPartial = map[ConditionalInitAliasQuant]func(){
	ConditionalInitAliasA: func() {}, ConditionalInitAliasC: func() {},
}
var conditionalInitAliasView = map[ConditionalInitAliasQuant]func(){
	ConditionalInitAliasA: func() {}, ConditionalInitAliasB: func() {}, ConditionalInitAliasC: func() {},
}
var conditionalInitAliasEnabled bool

func init() {
	if conditionalInitAliasEnabled {
		conditionalInitAliasView = conditionalInitAliasPartial
	}
}
func conditionalInitAliasAll(q ConditionalInitAliasQuant) bool {
	return q == ConditionalInitAliasA || q == ConditionalInitAliasB || q == ConditionalInitAliasC
}
func ConditionalInitAliasQMatMul(q ConditionalInitAliasQuant) bool {
	_, ok := conditionalInitAliasView[q]
	return ok && conditionalInitAliasAll(q)
}

// Deferred package-map writes execute after later source-order assignments and
// therefore keep entry alias state unknown.
type DeferredInitAliasQuant uint8

const (
	DeferredInitAliasA DeferredInitAliasQuant = iota
	DeferredInitAliasB
	DeferredInitAliasC
)

func deferredInitAliasByteSize(q DeferredInitAliasQuant) int {
	switch q {
	case DeferredInitAliasA, DeferredInitAliasB, DeferredInitAliasC:
		return 8
	default:
		return 0
	}
}
func deferredInitAliasDecode(q DeferredInitAliasQuant) []float32 {
	switch q {
	case DeferredInitAliasA, DeferredInitAliasB, DeferredInitAliasC:
		return []float32{}
	default:
		return nil
	}
}

var deferredInitAliasPartial = map[DeferredInitAliasQuant]func(){
	DeferredInitAliasA: func() {}, DeferredInitAliasC: func() {},
}
var deferredInitAliasComplete = map[DeferredInitAliasQuant]func(){
	DeferredInitAliasA: func() {}, DeferredInitAliasB: func() {}, DeferredInitAliasC: func() {},
}
var deferredInitAliasView = deferredInitAliasPartial

func setDeferredInitAliasFull() { deferredInitAliasView = deferredInitAliasComplete }
func init() {
	defer setDeferredInitAliasFull()
	deferredInitAliasView = deferredInitAliasPartial
}
func deferredInitAliasAll(q DeferredInitAliasQuant) bool {
	return q == DeferredInitAliasA || q == DeferredInitAliasB || q == DeferredInitAliasC
}
func DeferredInitAliasQMatMul(q DeferredInitAliasQuant) bool {
	_, ok := deferredInitAliasView[q]
	return ok && deferredInitAliasAll(q)
}

// Stable global dispatch tables indexed inside invoked literals remain visible.
type LiteralGlobalQuant uint8

const (
	LiteralGlobalA LiteralGlobalQuant = iota
	LiteralGlobalB                    // want `quant variant LiteralGlobalB \(LiteralGlobalQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	LiteralGlobalC
)

func literalGlobalByteSize(q LiteralGlobalQuant) int {
	switch q {
	case LiteralGlobalA, LiteralGlobalB, LiteralGlobalC:
		return 8
	default:
		return 0
	}
}
func literalGlobalDecode(q LiteralGlobalQuant) []float32 {
	switch q {
	case LiteralGlobalA, LiteralGlobalB, LiteralGlobalC:
		return []float32{}
	default:
		return nil
	}
}

var literalGlobalDispatch = map[LiteralGlobalQuant]func(){
	LiteralGlobalA: func() {}, LiteralGlobalC: func() {},
}

func literalGlobalNamed(q LiteralGlobalQuant) bool {
	return q == LiteralGlobalA || q == LiteralGlobalC
}
func LiteralGlobalQMatMul(q LiteralGlobalQuant) bool {
	lookup := func() bool {
		_ = func() {
			literalGlobalDispatch[LiteralGlobalB] = func() {}
		}
		_, ok := literalGlobalDispatch[q]
		return ok
		literalGlobalDispatch[LiteralGlobalB] = func() {}
		return false
	}
	return lookup() && literalGlobalNamed(q)
}

// Read-only is constant-scoped only, and prose containing a directive token
// is not itself a directive.
type DirectiveQuant uint8

const (
	DirectiveA DirectiveQuant = iota
	// This is not perfscan:quant-matmul-coverage-validated.
	DirectiveB // want `quant variant DirectiveB \(DirectiveQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	DirectiveC
)

func directiveByteSize(q DirectiveQuant) int {
	switch q {
	case DirectiveA, DirectiveB, DirectiveC:
		return 8
	default:
		return 0
	}
}
func directiveDecode(q DirectiveQuant) []float32 {
	switch q {
	case DirectiveA, DirectiveB, DirectiveC:
		return []float32{}
	default:
		return nil
	}
}
func directiveOne(q DirectiveQuant) bool { return q == DirectiveA || q == DirectiveC }
func directiveTwo(q DirectiveQuant) bool { return q == DirectiveA || q == DirectiveC }

// perfscan:quant-matmul-read-only is invalid on a dispatch function.
func DirectiveQMatMul(q DirectiveQuant) bool { return directiveOne(q) && directiveTwo(q) }

// Tagless switches and typed getter tags retain their dynamic enum subject.
type TaglessQuant uint8

const (
	TaglessA TaglessQuant = iota
	TaglessB              // want `quant variant TaglessB \(TaglessQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	TaglessC
)

func taglessByteSize(q TaglessQuant) int {
	switch q {
	case TaglessA, TaglessB, TaglessC:
		return 8
	default:
		return 0
	}
}
func taglessDecode(q TaglessQuant) []float32 {
	switch q {
	case TaglessA, TaglessB, TaglessC:
		return []float32{}
	default:
		return nil
	}
}
func taglessOne(q TaglessQuant) bool {
	switch {
	case q == TaglessA, q == TaglessC:
		return true
	default:
		return false
	}
}
func currentTagless(q TaglessQuant) TaglessQuant { return q }
func taglessTwo(q TaglessQuant) bool {
	switch currentTagless(q) {
	case TaglessA, TaglessC:
		return true
	default:
		return false
	}
}
func TaglessQMatMul(q TaglessQuant) bool { return taglessOne(q) && taglessTwo(q) }

// Boolean predicates use union/intersection/complement semantics rather than
// independently collecting every mentioned constant.
type AlgebraQuant uint8

const (
	AlgebraA AlgebraQuant = iota
	AlgebraB              // want `quant variant AlgebraB \(AlgebraQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	AlgebraC
)

func algebraByteSize(q AlgebraQuant) int {
	switch q {
	case AlgebraA, AlgebraB, AlgebraC:
		return 8
	default:
		return 0
	}
}
func algebraDecode(q AlgebraQuant) []float32 {
	switch q {
	case AlgebraA, AlgebraB, AlgebraC:
		return []float32{}
	default:
		return nil
	}
}
func algebraPartial(q AlgebraQuant) bool   { return q == AlgebraA || q != AlgebraB }
func algebraUniversal(q AlgebraQuant) bool { return q != AlgebraA || q != AlgebraB }
func AlgebraQMatMul(q AlgebraQuant) bool   { return algebraPartial(q) && algebraUniversal(q) }

// A typed nil conversion is still a nil callable map entry.
type NilHandlerQuant uint8

const (
	NilHandlerA NilHandlerQuant = iota
	NilHandlerB                 // want `quant variant NilHandlerB \(NilHandlerQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	NilHandlerC
)

func nilHandlerByteSize(q NilHandlerQuant) int {
	switch q {
	case NilHandlerA, NilHandlerB, NilHandlerC:
		return 8
	default:
		return 0
	}
}
func nilHandlerDecode(q NilHandlerQuant) []float32 {
	switch q {
	case NilHandlerA, NilHandlerB, NilHandlerC:
		return []float32{}
	default:
		return nil
	}
}
func nilHandlerOK() {}
func NilHandlerQMatMul(q NilHandlerQuant) bool {
	one := map[NilHandlerQuant]func(){NilHandlerA: nilHandlerOK, NilHandlerB: (func())(nil), NilHandlerC: nilHandlerOK}
	two := map[NilHandlerQuant]func(){NilHandlerA: nilHandlerOK, NilHandlerB: (func())(nil), NilHandlerC: nilHandlerOK}
	return one[q] != nil && two[q] != nil
}

// A directly reached dequant helper is a CPU dispatch layer even though its
// own name also supplies portable decode evidence.
type DecodeLayerQuant uint8

const (
	DecodeLayerA DecodeLayerQuant = iota
	DecodeLayerB                  // want `quant variant DecodeLayerB \(DecodeLayerQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	DecodeLayerC
)

func layerByteSize(q DecodeLayerQuant) int {
	switch q {
	case DecodeLayerA, DecodeLayerB, DecodeLayerC:
		return 8
	default:
		return 0
	}
}
func decodeLayerPortableDecode(q DecodeLayerQuant) []float32 {
	switch q {
	case DecodeLayerA, DecodeLayerB, DecodeLayerC:
		return []float32{}
	default:
		return nil
	}
}
func dequantizeLayerCPU(q DecodeLayerQuant) bool {
	switch q {
	case DecodeLayerA, DecodeLayerC:
		return true
	default:
		return false
	}
}
func decodeLayerSupported(q DecodeLayerQuant) bool { return q == DecodeLayerA || q == DecodeLayerC }
func DecodeLayerQMatMul(q DecodeLayerQuant) bool {
	return decodeLayerSupported(q) && dequantizeLayerCPU(q)
}

// Suppression follows an explicit raw-ID/public-enum alias relation.
const (
	rawSuppressedA = 201
	rawSuppressedB = 202
	rawSuppressedC = 203
)

type RawSuppressedQuant uint16

const (
	RawSuppressedA RawSuppressedQuant = rawSuppressedA
	RawSuppressedB RawSuppressedQuant = rawSuppressedB //perfscan:quant-matmul-read-only external-only format.
	RawSuppressedC RawSuppressedQuant = rawSuppressedC
)

func rawSuppressedByteSize(raw uint16) int {
	switch raw {
	case rawSuppressedA, rawSuppressedB, rawSuppressedC:
		return 8
	default:
		return 0
	}
}
func rawSuppressedDecode(q RawSuppressedQuant) []float32 {
	switch q {
	case rawSuppressedA, rawSuppressedB, rawSuppressedC:
		return []float32{}
	default:
		return nil
	}
}
func rawSuppressedOne(q RawSuppressedQuant) bool { return q == rawSuppressedA || q == rawSuppressedC }
func rawSuppressedTwo(q RawSuppressedQuant) bool { return q == rawSuppressedA || q == rawSuppressedC }
func RawSuppressedQMatMul(q RawSuppressedQuant) bool {
	return rawSuppressedOne(q) && rawSuppressedTwo(q)
}

// Conditional callback calls retain their internal fixed argument scope.
type CallbackProjectionQuant uint8

const (
	CallbackProjectionA CallbackProjectionQuant = iota // want `quant variant CallbackProjectionA \(CallbackProjectionQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	CallbackProjectionB
	CallbackProjectionC
)

func callbackProjectionByteSize(q CallbackProjectionQuant) int {
	switch q {
	case CallbackProjectionA, CallbackProjectionB, CallbackProjectionC:
		return 8
	default:
		return 0
	}
}
func callbackProjectionDecode(q CallbackProjectionQuant) []float32 {
	switch q {
	case CallbackProjectionA, CallbackProjectionB, CallbackProjectionC:
		return []float32{}
	default:
		return nil
	}
}

var callbackProjectionEnabled bool

func callbackProjectionPartial(q CallbackProjectionQuant) bool {
	return q == CallbackProjectionB || q == CallbackProjectionC
}
func callbackProjectionComplete(q CallbackProjectionQuant) bool {
	return q == CallbackProjectionA || q == CallbackProjectionB || q == CallbackProjectionC
}
func callbackProjectionDispatch(callback func(CallbackProjectionQuant) bool, _ CallbackProjectionQuant) bool {
	if callbackProjectionEnabled {
		return callback(CallbackProjectionA)
	}
	return true
}
func CallbackProjectionQMatMul(q CallbackProjectionQuant) bool {
	return callbackProjectionComplete(q) && callbackProjectionDispatch(callbackProjectionPartial, q)
}

// Canonical global-table sites preserve the scopes of every invoked literal.
type LiteralRouteQuant uint8

const (
	LiteralRouteA LiteralRouteQuant = iota
	LiteralRouteB                   // want `quant variant LiteralRouteB \(LiteralRouteQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	LiteralRouteC
)

func literalRouteByteSize(q LiteralRouteQuant) int {
	switch q {
	case LiteralRouteA, LiteralRouteB, LiteralRouteC:
		return 8
	default:
		return 0
	}
}
func literalRouteDecode(q LiteralRouteQuant) []float32 {
	switch q {
	case LiteralRouteA, LiteralRouteB, LiteralRouteC:
		return []float32{}
	default:
		return nil
	}
}

var literalRouteTable = map[LiteralRouteQuant]func(){
	LiteralRouteA: func() {}, LiteralRouteC: func() {},
}

func literalRouteComplete(q LiteralRouteQuant) bool {
	return q == LiteralRouteA || q == LiteralRouteB || q == LiteralRouteC
}
func LiteralRouteQMatMul(q LiteralRouteQuant) bool {
	first := func(value LiteralRouteQuant) bool {
		_, ok := literalRouteTable[value]
		return ok
	}
	second := func(value LiteralRouteQuant) bool {
		_, ok := literalRouteTable[value]
		return ok
	}
	return literalRouteComplete(q) && first(LiteralRouteA) && second(LiteralRouteB)
}

// Deferred invoked literals contribute their reachable CPU layer.
type DeferredLiteralQuant uint8

const (
	DeferredLiteralA DeferredLiteralQuant = iota
	DeferredLiteralB                      // want `quant variant DeferredLiteralB \(DeferredLiteralQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	DeferredLiteralC
)

func deferredLiteralByteSize(q DeferredLiteralQuant) int {
	switch q {
	case DeferredLiteralA, DeferredLiteralB, DeferredLiteralC:
		return 8
	default:
		return 0
	}
}
func deferredLiteralDecode(q DeferredLiteralQuant) []float32 {
	switch q {
	case DeferredLiteralA, DeferredLiteralB, DeferredLiteralC:
		return []float32{}
	default:
		return nil
	}
}
func deferredLiteralPartial(q DeferredLiteralQuant) bool {
	return q == DeferredLiteralA || q == DeferredLiteralC
}
func deferredLiteralComplete(q DeferredLiteralQuant) bool {
	return q == DeferredLiteralA || q == DeferredLiteralB || q == DeferredLiteralC
}
func DeferredLiteralQMatMul(q DeferredLiteralQuant) bool {
	defer func() { _ = deferredLiteralPartial(q) }()
	return deferredLiteralComplete(q)
}

// Forwarded named-function aliases do not become universal matmul roots.
type ForwardedAliasQuant uint8

const (
	ForwardedAliasA ForwardedAliasQuant = iota
	ForwardedAliasB
	ForwardedAliasC
)

func forwardedAliasByteSize(q ForwardedAliasQuant) int {
	switch q {
	case ForwardedAliasA, ForwardedAliasB, ForwardedAliasC:
		return 8
	default:
		return 0
	}
}
func forwardedAliasDecode(q ForwardedAliasQuant) []float32 {
	switch q {
	case ForwardedAliasA, ForwardedAliasB, ForwardedAliasC:
		return []float32{}
	default:
		return nil
	}
}
func forwardedAliasPartial(q ForwardedAliasQuant) bool {
	return q == ForwardedAliasA || q == ForwardedAliasC
}
func forwardedAliasComplete(q ForwardedAliasQuant) bool {
	return q == ForwardedAliasA || q == ForwardedAliasB || q == ForwardedAliasC
}
func forwardedAliasDispatch(callback func(ForwardedAliasQuant) bool, q ForwardedAliasQuant) bool {
	return callback(q)
}
func forwardedAliasFixedDispatch(callback func(ForwardedAliasQuant) bool) bool {
	return callback(ForwardedAliasA)
}
func ForwardedAliasQMatMul(q ForwardedAliasQuant) bool {
	callback := forwardedAliasPartial
	return forwardedAliasComplete(q) && forwardedAliasDispatch(callback, ForwardedAliasA) &&
		forwardedAliasFixedDispatch(callback)
}

// Initializers in enclosing statement blocks preserve fixed caller scope.
type NestedAliasQuant uint8

const (
	NestedAliasA NestedAliasQuant = iota
	NestedAliasB
	NestedAliasC
)

func nestedAliasByteSize(q NestedAliasQuant) int {
	switch q {
	case NestedAliasA, NestedAliasB, NestedAliasC:
		return 8
	default:
		return 0
	}
}
func nestedAliasDecode(q NestedAliasQuant) []float32 {
	switch q {
	case NestedAliasA, NestedAliasB, NestedAliasC:
		return []float32{}
	default:
		return nil
	}
}

var nestedAliasEnabled bool

func nestedAliasPartial(q NestedAliasQuant) bool { return q == NestedAliasA || q == NestedAliasC }
func nestedAliasComplete(q NestedAliasQuant) bool {
	return q == NestedAliasA || q == NestedAliasB || q == NestedAliasC
}
func nestedAliasLayer(q NestedAliasQuant) bool {
	alias := q
	if nestedAliasEnabled {
		return nestedAliasPartial(alias)
	}
	return true
}
func NestedAliasQMatMul(q NestedAliasQuant) bool {
	return nestedAliasComplete(q) && nestedAliasLayer(NestedAliasA)
}

// Snapshot aliases stop following a source that is reassigned later.
type SnapshotAliasQuant uint8

const (
	SnapshotAliasA SnapshotAliasQuant = iota // want `quant variant SnapshotAliasA \(SnapshotAliasQuant\).*absent from 1 of 3 reachable CPU matmul dispatch layers`
	SnapshotAliasB
	SnapshotAliasC
)

func snapshotAliasByteSize(q SnapshotAliasQuant) int {
	switch q {
	case SnapshotAliasA, SnapshotAliasB, SnapshotAliasC:
		return 8
	default:
		return 0
	}
}
func snapshotAliasDecode(q SnapshotAliasQuant) []float32 {
	switch q {
	case SnapshotAliasA, SnapshotAliasB, SnapshotAliasC:
		return []float32{}
	default:
		return nil
	}
}
func snapshotAliasPartial(q SnapshotAliasQuant) bool { return q != SnapshotAliasA }
func snapshotAliasComplete(q SnapshotAliasQuant) bool {
	return q == SnapshotAliasA || q == SnapshotAliasB || q == SnapshotAliasC
}
func snapshotAliasLayer(q SnapshotAliasQuant) bool {
	alias := q
	q = SnapshotAliasB
	if q == SnapshotAliasB {
		return snapshotAliasPartial(alias)
	}
	return true
}
func SnapshotAliasQMatMul(q SnapshotAliasQuant) bool {
	return snapshotAliasComplete(q) && snapshotAliasLayer(q)
}

// Mutating condition operands prevent stale enum narrowing.
type MutatingConditionQuant uint8

const (
	MutatingConditionA MutatingConditionQuant = iota
	MutatingConditionB                        // want `quant variant MutatingConditionB \(MutatingConditionQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	MutatingConditionC
)

func mutatingConditionByteSize(q MutatingConditionQuant) int {
	switch q {
	case MutatingConditionA, MutatingConditionB, MutatingConditionC:
		return 8
	default:
		return 0
	}
}
func mutatingConditionDecode(q MutatingConditionQuant) []float32 {
	switch q {
	case MutatingConditionA, MutatingConditionB, MutatingConditionC:
		return []float32{}
	default:
		return nil
	}
}
func mutatingConditionSetB(q *MutatingConditionQuant) bool {
	*q = MutatingConditionB
	return true
}
func mutatingConditionPartial(q MutatingConditionQuant) bool {
	return q == MutatingConditionA || q == MutatingConditionC
}
func mutatingConditionComplete(q MutatingConditionQuant) bool {
	return q == MutatingConditionA || q == MutatingConditionB || q == MutatingConditionC
}
func mutatingConditionLayer(q MutatingConditionQuant) bool {
	if q == MutatingConditionA && mutatingConditionSetB(&q) {
		return mutatingConditionPartial(q)
	}
	return true
}
func MutatingConditionQMatMul(q MutatingConditionQuant) bool {
	return mutatingConditionComplete(q) && mutatingConditionLayer(MutatingConditionA)
}

// Dead references do not promote precisely reached helpers to universal roots.
type DeadReferenceQuant uint8

const (
	DeadReferenceA DeadReferenceQuant = iota
	DeadReferenceB
	DeadReferenceC
)

func deadReferenceByteSize(q DeadReferenceQuant) int {
	switch q {
	case DeadReferenceA, DeadReferenceB, DeadReferenceC:
		return 8
	default:
		return 0
	}
}
func deadReferenceDecode(q DeadReferenceQuant) []float32 {
	switch q {
	case DeadReferenceA, DeadReferenceB, DeadReferenceC:
		return []float32{}
	default:
		return nil
	}
}
func deadReferencePartial(q DeadReferenceQuant) bool {
	return q == DeadReferenceA || q == DeadReferenceC
}
func deadReferenceComplete(q DeadReferenceQuant) bool {
	return q == DeadReferenceA || q == DeadReferenceB || q == DeadReferenceC
}
func DeadReferenceQMatMul(q DeadReferenceQuant) bool {
	if false {
		_ = deadReferencePartial
	}
	_ = func() { _ = deadReferencePartial }
	return deadReferenceComplete(q) && deadReferencePartial(DeadReferenceA)
}

// Conditional callback summaries propagate through named forwarding helpers.
type TransitiveCallbackQuant uint8

const (
	TransitiveCallbackA TransitiveCallbackQuant = iota // want `quant variant TransitiveCallbackA \(TransitiveCallbackQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	TransitiveCallbackB
	TransitiveCallbackC
)

func transitiveCallbackByteSize(q TransitiveCallbackQuant) int {
	switch q {
	case TransitiveCallbackA, TransitiveCallbackB, TransitiveCallbackC:
		return 8
	default:
		return 0
	}
}
func transitiveCallbackDecode(q TransitiveCallbackQuant) []float32 {
	switch q {
	case TransitiveCallbackA, TransitiveCallbackB, TransitiveCallbackC:
		return []float32{}
	default:
		return nil
	}
}

var transitiveCallbackEnabled bool

func transitiveCallbackPartial(q TransitiveCallbackQuant) bool { return q != TransitiveCallbackA }
func transitiveCallbackComplete(q TransitiveCallbackQuant) bool {
	return q == TransitiveCallbackA || q == TransitiveCallbackB || q == TransitiveCallbackC
}
func transitiveCallbackInner(callback func(TransitiveCallbackQuant) bool, _ TransitiveCallbackQuant) bool {
	if transitiveCallbackEnabled {
		return callback(TransitiveCallbackA)
	}
	return true
}
func transitiveCallbackOuter(callback func(TransitiveCallbackQuant) bool, q TransitiveCallbackQuant) bool {
	return transitiveCallbackInner(callback, q)
}
func TransitiveCallbackQMatMul(q TransitiveCallbackQuant) bool {
	return transitiveCallbackComplete(q) && transitiveCallbackOuter(transitiveCallbackPartial, q)
}

// Anonymous callbacks are invoked and scoped through conditional dispatchers.
type AnonymousCallbackQuant uint8

const (
	AnonymousCallbackA AnonymousCallbackQuant = iota // want `quant variant AnonymousCallbackA \(AnonymousCallbackQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	AnonymousCallbackB
	AnonymousCallbackC
)

func anonymousCallbackByteSize(q AnonymousCallbackQuant) int {
	switch q {
	case AnonymousCallbackA, AnonymousCallbackB, AnonymousCallbackC:
		return 8
	default:
		return 0
	}
}
func anonymousCallbackDecode(q AnonymousCallbackQuant) []float32 {
	switch q {
	case AnonymousCallbackA, AnonymousCallbackB, AnonymousCallbackC:
		return []float32{}
	default:
		return nil
	}
}

var anonymousCallbackEnabled bool

func anonymousCallbackPartial(q AnonymousCallbackQuant) bool { return q != AnonymousCallbackA }
func anonymousCallbackComplete(q AnonymousCallbackQuant) bool {
	return q == AnonymousCallbackA || q == AnonymousCallbackB || q == AnonymousCallbackC
}
func anonymousCallbackDispatch(callback func(AnonymousCallbackQuant) bool, _ AnonymousCallbackQuant) bool {
	if anonymousCallbackEnabled {
		return callback(AnonymousCallbackA)
	}
	return true
}
func AnonymousCallbackQMatMul(q AnonymousCallbackQuant) bool {
	return anonymousCallbackComplete(q) && anonymousCallbackDispatch(
		func(value AnonymousCallbackQuant) bool { return anonymousCallbackPartial(value) }, q,
	)
}

// Named integer parameters without declared constants do not erase other enum edges.
type DomainlessMode int
type DomainlessQuant uint8

const (
	DomainlessA DomainlessQuant = iota
	DomainlessB                 // want `quant variant DomainlessB \(DomainlessQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	DomainlessC
)

func domainlessByteSize(q DomainlessQuant) int {
	switch q {
	case DomainlessA, DomainlessB, DomainlessC:
		return 8
	default:
		return 0
	}
}
func domainlessDecode(q DomainlessQuant) []float32 {
	switch q {
	case DomainlessA, DomainlessB, DomainlessC:
		return []float32{}
	default:
		return nil
	}
}
func domainlessPartial(q DomainlessQuant) bool { return q == DomainlessA || q == DomainlessC }
func domainlessComplete(q DomainlessQuant) bool {
	return q == DomainlessA || q == DomainlessB || q == DomainlessC
}
func domainlessLayer(_ DomainlessMode, q DomainlessQuant) bool { return domainlessPartial(q) }
func DomainlessQMatMul(mode DomainlessMode, q DomainlessQuant) bool {
	return domainlessComplete(q) && domainlessLayer(mode, q)
}

// Fallthrough writes invalidate a constrained entry value.
type FallthroughMutationQuant uint8

const (
	FallthroughMutationA FallthroughMutationQuant = iota
	FallthroughMutationB                          // want `quant variant FallthroughMutationB \(FallthroughMutationQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	FallthroughMutationC
)

func fallthroughMutationByteSize(q FallthroughMutationQuant) int {
	switch q {
	case FallthroughMutationA, FallthroughMutationB, FallthroughMutationC:
		return 8
	default:
		return 0
	}
}
func fallthroughMutationDecode(q FallthroughMutationQuant) []float32 {
	switch q {
	case FallthroughMutationA, FallthroughMutationB, FallthroughMutationC:
		return []float32{}
	default:
		return nil
	}
}
func fallthroughMutationPartial(q FallthroughMutationQuant) bool {
	return q == FallthroughMutationA || q == FallthroughMutationC
}
func fallthroughMutationComplete(q FallthroughMutationQuant) bool {
	return q == FallthroughMutationA || q == FallthroughMutationB || q == FallthroughMutationC
}
func fallthroughMutationLayer(q FallthroughMutationQuant) bool {
	switch q {
	case FallthroughMutationA:
		q = FallthroughMutationB
		fallthrough
	case FallthroughMutationB:
		return fallthroughMutationPartial(q)
	}
	return true
}
func FallthroughMutationQMatMul(q FallthroughMutationQuant) bool {
	return fallthroughMutationComplete(q) && fallthroughMutationLayer(FallthroughMutationA)
}

// Function conversions, method values, and repeated assignment targets retain fixed scope.
type CallableAliasQuant uint8

const (
	CallableAliasA CallableAliasQuant = iota
	CallableAliasB
	CallableAliasC
)

func callableAliasByteSize(q CallableAliasQuant) int {
	switch q {
	case CallableAliasA, CallableAliasB, CallableAliasC:
		return 8
	default:
		return 0
	}
}
func callableAliasDecode(q CallableAliasQuant) []float32 {
	switch q {
	case CallableAliasA, CallableAliasB, CallableAliasC:
		return []float32{}
	default:
		return nil
	}
}

type callableAliasFunction func(CallableAliasQuant) bool
type callableAliasReceiver struct{}

func callableAliasMatmulPartial(q CallableAliasQuant) bool {
	return q == CallableAliasA || q == CallableAliasC
}
func (callableAliasReceiver) callableAliasMatmulMethod(q CallableAliasQuant) bool {
	return q == CallableAliasA || q == CallableAliasC
}
func callableAliasComplete(q CallableAliasQuant) bool {
	return q == CallableAliasA || q == CallableAliasB || q == CallableAliasC
}
func callableAliasRepeated(q CallableAliasQuant) bool {
	alias := q
	alias, alias = q, CallableAliasA
	return callableAliasMatmulPartial(alias)
}
func CallableAliasQMatMul(q CallableAliasQuant) bool {
	converted := callableAliasFunction(callableAliasMatmulPartial)
	receiver := callableAliasReceiver{}
	method := receiver.callableAliasMatmulMethod
	return callableAliasComplete(q) && converted(CallableAliasA) && method(CallableAliasA) &&
		callableAliasRepeated(CallableAliasB)
}

// Callback method expressions map their receiver, while bound enum receivers retain fixed scope.
type MethodCallbackQuant uint8

const (
	MethodCallbackA MethodCallbackQuant = iota
	MethodCallbackB                     // want `quant variant MethodCallbackB \(MethodCallbackQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	MethodCallbackC
)

func methodCallbackByteSize(q MethodCallbackQuant) int {
	switch q {
	case MethodCallbackA, MethodCallbackB, MethodCallbackC:
		return 8
	default:
		return 0
	}
}
func methodCallbackDecode(q MethodCallbackQuant) []float32 {
	switch q {
	case MethodCallbackA, MethodCallbackB, MethodCallbackC:
		return []float32{}
	default:
		return nil
	}
}

type methodCallbackWorker struct{}

func (methodCallbackWorker) apply(q MethodCallbackQuant) bool {
	return q == MethodCallbackA || q == MethodCallbackC
}
func (receiver MethodCallbackQuant) boundApply(_ MethodCallbackQuant) bool {
	return receiver == MethodCallbackA || receiver == MethodCallbackC
}
func (receiver MethodCallbackQuant) snapshotApply(_ MethodCallbackQuant) bool {
	return receiver == MethodCallbackA
}
func methodCallbackDispatch(
	callback func(methodCallbackWorker, MethodCallbackQuant) bool,
	worker methodCallbackWorker,
	q MethodCallbackQuant,
) bool {
	return callback(worker, q)
}
func methodCallbackBoundDispatch(
	callback func(MethodCallbackQuant) bool,
	q MethodCallbackQuant,
) bool {
	return callback(q)
}
func methodCallbackComplete(q MethodCallbackQuant) bool {
	return q == MethodCallbackA || q == MethodCallbackB || q == MethodCallbackC
}
func MethodCallbackQMatMul(q MethodCallbackQuant) bool {
	worker := methodCallbackWorker{}
	direct := MethodCallbackA.boundApply
	receiver := MethodCallbackA
	snapshot := receiver.snapshotApply
	receiver = MethodCallbackB
	return methodCallbackComplete(q) &&
		methodCallbackDispatch(methodCallbackWorker.apply, worker, q) &&
		methodCallbackBoundDispatch(MethodCallbackA.boundApply, q) && direct(q) && snapshot(q)
}

// Dispatcher-local aliases, conversions, and invoked closures expose callback sites.
type DispatcherAliasQuant uint8

const (
	DispatcherAliasA DispatcherAliasQuant = iota // want `quant variant DispatcherAliasA \(DispatcherAliasQuant\).*absent from 3 of 4 reachable CPU matmul dispatch layers`
	DispatcherAliasB
	DispatcherAliasC
)

func dispatcherAliasByteSize(q DispatcherAliasQuant) int {
	switch q {
	case DispatcherAliasA, DispatcherAliasB, DispatcherAliasC:
		return 8
	default:
		return 0
	}
}
func dispatcherAliasDecode(q DispatcherAliasQuant) []float32 {
	switch q {
	case DispatcherAliasA, DispatcherAliasB, DispatcherAliasC:
		return []float32{}
	default:
		return nil
	}
}

type dispatcherAliasFunction func(DispatcherAliasQuant) bool
type dispatcherAliasThunk func() bool

var dispatcherAliasEnabled bool

func dispatcherAliasPartial(q DispatcherAliasQuant) bool { return q != DispatcherAliasA }
func dispatcherAliasNestedPartial(q DispatcherAliasQuant) bool {
	return q != DispatcherAliasA
}
func dispatcherAliasStoredPartial(q DispatcherAliasQuant) bool {
	return q != DispatcherAliasA
}
func dispatcherAliasComplete(q DispatcherAliasQuant) bool {
	return q == DispatcherAliasA || q == DispatcherAliasB || q == DispatcherAliasC
}
func dispatcherAliasLayer(callback func(DispatcherAliasQuant) bool) bool {
	alias := callback
	if dispatcherAliasEnabled {
		return dispatcherAliasFunction(alias)(DispatcherAliasA)
	}
	return true
}
func dispatcherAliasIIFE(callback func(DispatcherAliasQuant) bool) bool {
	return func() bool { return callback(DispatcherAliasA) }()
}
func dispatcherAliasNestedIIFE(callback func(DispatcherAliasQuant) bool) bool {
	return func() bool {
		return func() bool { return callback(DispatcherAliasA) }()
	}()
}
func dispatcherAliasStored(callback func(DispatcherAliasQuant) bool) bool {
	run := func() bool { return callback(DispatcherAliasA) }
	converted := dispatcherAliasThunk(run)
	return converted()
}
func DispatcherAliasQMatMul(q DispatcherAliasQuant) bool {
	return dispatcherAliasComplete(q) && dispatcherAliasLayer(dispatcherAliasPartial) &&
		dispatcherAliasIIFE(dispatcherAliasPartial) &&
		dispatcherAliasNestedIIFE(dispatcherAliasNestedPartial) &&
		dispatcherAliasStored(dispatcherAliasStoredPartial)
}

// Every statically possible named callback target remains CPU-reachable.
type MultiTargetCallbackQuant uint8

const (
	MultiTargetCallbackA MultiTargetCallbackQuant = iota // want `quant variant MultiTargetCallbackA \(MultiTargetCallbackQuant\).*absent from 3 of 4 reachable CPU matmul dispatch layers`
	MultiTargetCallbackB
	MultiTargetCallbackC
)

func multiTargetCallbackByteSize(q MultiTargetCallbackQuant) int {
	switch q {
	case MultiTargetCallbackA, MultiTargetCallbackB, MultiTargetCallbackC:
		return 8
	default:
		return 0
	}
}
func multiTargetCallbackDecode(q MultiTargetCallbackQuant) []float32 {
	switch q {
	case MultiTargetCallbackA, MultiTargetCallbackB, MultiTargetCallbackC:
		return []float32{}
	default:
		return nil
	}
}

var multiTargetCallbackFlag bool
var multiTargetCallbackReturn bool
var multiTargetCallbackConditional bool

func multiTargetCallbackPartialOne(q MultiTargetCallbackQuant) bool {
	return q != MultiTargetCallbackA
}
func multiTargetCallbackPartialTwo(q MultiTargetCallbackQuant) bool {
	return q != MultiTargetCallbackA
}
func multiTargetCallbackDormantPartial(q MultiTargetCallbackQuant) bool {
	return q != MultiTargetCallbackA
}
func multiTargetCallbackLatePartial(q MultiTargetCallbackQuant) bool {
	return q != MultiTargetCallbackA
}
func multiTargetCallbackReturningPartial(q MultiTargetCallbackQuant) bool {
	return q != MultiTargetCallbackA
}
func multiTargetCallbackConditionalPartial(q MultiTargetCallbackQuant) bool {
	return q != MultiTargetCallbackA
}
func multiTargetCallbackComplete(q MultiTargetCallbackQuant) bool {
	return q == MultiTargetCallbackA || q == MultiTargetCallbackB || q == MultiTargetCallbackC
}
func multiTargetCallbackDispatch(
	callback func(MultiTargetCallbackQuant) bool,
	q MultiTargetCallbackQuant,
) bool {
	return callback(q)
}
func MultiTargetCallbackQMatMul(q MultiTargetCallbackQuant) bool {
	var callback func(MultiTargetCallbackQuant) bool
	if multiTargetCallbackFlag {
		callback = multiTargetCallbackPartialOne
	} else {
		callback = multiTargetCallbackPartialTwo
	}
	_ = func() { callback = multiTargetCallbackDormantPartial }
	late := func() { callback = multiTargetCallbackLatePartial }
	conditional := func() {
		if multiTargetCallbackConditional {
			callback = multiTargetCallbackConditionalPartial
		}
	}
	conditional()
	if multiTargetCallbackReturn {
		callback = multiTargetCallbackReturningPartial
		return multiTargetCallbackComplete(q)
	}
	result := multiTargetCallbackComplete(q) && multiTargetCallbackDispatch(callback, q)
	late()
	return result
}

// Direct calls through multi-target function variables retain every possible CPU edge.
type MultiTargetDirectQuant uint8

const (
	MultiTargetDirectA MultiTargetDirectQuant = iota // want `quant variant MultiTargetDirectA \(MultiTargetDirectQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	MultiTargetDirectB
	MultiTargetDirectC
)

func multiTargetDirectByteSize(q MultiTargetDirectQuant) int {
	switch q {
	case MultiTargetDirectA, MultiTargetDirectB, MultiTargetDirectC:
		return 8
	default:
		return 0
	}
}
func multiTargetDirectDecode(q MultiTargetDirectQuant) []float32 {
	switch q {
	case MultiTargetDirectA, MultiTargetDirectB, MultiTargetDirectC:
		return []float32{}
	default:
		return nil
	}
}

var multiTargetDirectFlag bool

func multiTargetDirectPartialOne(q MultiTargetDirectQuant) bool {
	return q != MultiTargetDirectA
}
func multiTargetDirectPartialTwo(q MultiTargetDirectQuant) bool {
	return q != MultiTargetDirectA
}
func multiTargetDirectComplete(q MultiTargetDirectQuant) bool {
	return q == MultiTargetDirectA || q == MultiTargetDirectB || q == MultiTargetDirectC
}
func MultiTargetDirectQMatMul(q MultiTargetDirectQuant) bool {
	var target func(MultiTargetDirectQuant) bool
	if multiTargetDirectFlag {
		target = multiTargetDirectPartialOne
	} else {
		target = multiTargetDirectPartialTwo
	}
	return multiTargetDirectComplete(q) && target(q)
}

// Range-bound named function targets retain every loop-selected CPU edge.
type RangeFunctionQuant uint8

const (
	RangeFunctionA RangeFunctionQuant = iota // want `quant variant RangeFunctionA \(RangeFunctionQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	RangeFunctionB
	RangeFunctionC
)

func rangeFunctionByteSize(q RangeFunctionQuant) int {
	switch q {
	case RangeFunctionA, RangeFunctionB, RangeFunctionC:
		return 8
	default:
		return 0
	}
}
func rangeFunctionDecode(q RangeFunctionQuant) []float32 {
	switch q {
	case RangeFunctionA, RangeFunctionB, RangeFunctionC:
		return []float32{}
	default:
		return nil
	}
}

func rangeFunctionPartialOne(q RangeFunctionQuant) bool { return q != RangeFunctionA }
func rangeFunctionPartialTwo(q RangeFunctionQuant) bool { return q != RangeFunctionA }
func rangeFunctionComplete(q RangeFunctionQuant) bool {
	return q == RangeFunctionA || q == RangeFunctionB || q == RangeFunctionC
}
func RangeFunctionQMatMul(q RangeFunctionQuant) bool {
	for _, target := range []func(RangeFunctionQuant) bool{
		rangeFunctionPartialOne,
		rangeFunctionPartialTwo,
	} {
		if !target(q) {
			return false
		}
	}
	return rangeFunctionComplete(q)
}

// Exhaustive closure writes kill prior targets; address escapes make local targets unknown.
type CallableFlowQuant uint8

const (
	CallableFlowA CallableFlowQuant = iota
	CallableFlowB
	CallableFlowC
)

func callableFlowByteSize(q CallableFlowQuant) int {
	switch q {
	case CallableFlowA, CallableFlowB, CallableFlowC:
		return 8
	default:
		return 0
	}
}
func callableFlowDecode(q CallableFlowQuant) []float32 {
	switch q {
	case CallableFlowA, CallableFlowB, CallableFlowC:
		return []float32{}
	default:
		return nil
	}
}

var callableFlowFlag bool

func callableFlowPartial(q CallableFlowQuant) bool { return q != CallableFlowA }
func callableFlowCompleteOne(q CallableFlowQuant) bool {
	return q == CallableFlowA || q == CallableFlowB || q == CallableFlowC
}
func callableFlowCompleteTwo(q CallableFlowQuant) bool {
	return q == CallableFlowA || q == CallableFlowB || q == CallableFlowC
}
func callableFlowOverwrite(target *func(CallableFlowQuant) bool) {
	*target = callableFlowCompleteOne
}
func CallableFlowExhaustiveQMatMul(q CallableFlowQuant) bool {
	target := callableFlowPartial
	overwrite := func() {
		if callableFlowFlag {
			target = callableFlowCompleteOne
		} else {
			target = callableFlowCompleteTwo
		}
	}
	overwrite()
	return callableFlowCompleteOne(q) && target(q)
}
func CallableFlowAddressQMatMul(q CallableFlowQuant) bool {
	target := callableFlowPartial
	callableFlowOverwrite(&target)
	return callableFlowCompleteTwo(q) && target(q)
}

// Deferred callback writes occur after dispatch and cannot replace the current target.
type ScheduledCallableQuant uint8

const (
	ScheduledCallableA ScheduledCallableQuant = iota // want `quant variant ScheduledCallableA \(ScheduledCallableQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	ScheduledCallableB
	ScheduledCallableC
)

func scheduledCallableByteSize(q ScheduledCallableQuant) int {
	switch q {
	case ScheduledCallableA, ScheduledCallableB, ScheduledCallableC:
		return 8
	default:
		return 0
	}
}
func scheduledCallableDecode(q ScheduledCallableQuant) []float32 {
	switch q {
	case ScheduledCallableA, ScheduledCallableB, ScheduledCallableC:
		return []float32{}
	default:
		return nil
	}
}
func scheduledCallablePartialOne(q ScheduledCallableQuant) bool {
	return q != ScheduledCallableA
}
func scheduledCallablePartialTwo(q ScheduledCallableQuant) bool {
	return q != ScheduledCallableA
}
func scheduledCallableComplete(q ScheduledCallableQuant) bool {
	return q == ScheduledCallableA || q == ScheduledCallableB || q == ScheduledCallableC
}
func scheduledCallableDispatch(callback func(ScheduledCallableQuant) bool, q ScheduledCallableQuant) bool {
	return callback(q)
}
func ScheduledCallableQMatMul(q ScheduledCallableQuant) bool {
	target := scheduledCallablePartialOne
	defer func() { target = scheduledCallablePartialTwo }()
	return scheduledCallableComplete(q) && scheduledCallableDispatch(target, q)
}

// Literal parameters and factory returns resolve to their actual named callable targets.
type ResolvedCallableQuant uint8

const (
	ResolvedCallableA ResolvedCallableQuant = iota // want `quant variant ResolvedCallableA \(ResolvedCallableQuant\).*absent from 10 of 11 reachable CPU matmul dispatch layers`
	ResolvedCallableB
	ResolvedCallableC
)

func resolvedCallableByteSize(q ResolvedCallableQuant) int {
	switch q {
	case ResolvedCallableA, ResolvedCallableB, ResolvedCallableC:
		return 8
	default:
		return 0
	}
}
func resolvedCallableDecode(q ResolvedCallableQuant) []float32 {
	switch q {
	case ResolvedCallableA, ResolvedCallableB, ResolvedCallableC:
		return []float32{}
	default:
		return nil
	}
}

type resolvedCallableFunction func(ResolvedCallableQuant) bool

func resolvedCallableParameterPartial(q ResolvedCallableQuant) bool {
	return q != ResolvedCallableA
}
func resolvedCallableFactoryPartial(q ResolvedCallableQuant) bool {
	return q != ResolvedCallableA
}
func resolvedCallableNamedPartial(q ResolvedCallableQuant) bool {
	return q != ResolvedCallableA
}
func resolvedCallableConditionalPartial(q ResolvedCallableQuant) bool {
	return q != ResolvedCallableA
}
func resolvedCallableTuplePartial(q ResolvedCallableQuant) bool {
	return q != ResolvedCallableA
}
func resolvedCallableGenericPartial(q ResolvedCallableQuant) bool {
	return q != ResolvedCallableA
}
func resolvedCallableMethodPartial(q ResolvedCallableQuant) bool {
	return q != ResolvedCallableA
}
func resolvedCallableForwardPartial(q ResolvedCallableQuant) bool {
	return q != ResolvedCallableA
}
func resolvedCallableTupleArgumentPartial(q ResolvedCallableQuant) bool {
	return q != ResolvedCallableA
}
func resolvedCallableDeadPartial(q ResolvedCallableQuant) bool {
	return q != ResolvedCallableA
}
func resolvedCallableComplete(q ResolvedCallableQuant) bool {
	return q == ResolvedCallableA || q == ResolvedCallableB || q == ResolvedCallableC
}
func resolvedCallableFactory() func(ResolvedCallableQuant) bool {
	if false {
		return resolvedCallableDeadPartial
	}
	return resolvedCallableFactoryPartial
}

var resolvedCallableFlag bool

func resolvedCallableNamedFactory() (result func(ResolvedCallableQuant) bool) {
	result = resolvedCallableNamedPartial
	return
}
func resolvedCallableConditionalFactory(
	callback func(ResolvedCallableQuant) bool,
) (result func(ResolvedCallableQuant) bool) {
	if resolvedCallableFlag {
		result = callback
	} else {
		result = resolvedCallableComplete
	}
	return
}
func resolvedCallableTupleFactory() (func(ResolvedCallableQuant) bool, error) {
	return resolvedCallableTuplePartial, nil
}
func resolvedCallableGenericFactory[T any]() func(ResolvedCallableQuant) bool {
	return resolvedCallableGenericPartial
}

type resolvedCallableReceiver struct{}

func (resolvedCallableReceiver) factory(
	callback func(ResolvedCallableQuant) bool,
) func(ResolvedCallableQuant) bool {
	return callback
}
func resolvedCallableForwardBase() (bool, func(ResolvedCallableQuant) bool) {
	return true, resolvedCallableForwardPartial
}
func resolvedCallableForwardFactory() (bool, func(ResolvedCallableQuant) bool) {
	return resolvedCallableForwardBase()
}
func resolvedCallableTupleArgumentBase() (bool, func(ResolvedCallableQuant) bool) {
	return true, resolvedCallableTupleArgumentPartial
}
func resolvedCallableChoose(
	_ bool,
	callback func(ResolvedCallableQuant) bool,
) func(ResolvedCallableQuant) bool {
	return callback
}
func resolvedCallableLiteralFactory() func(ResolvedCallableQuant) bool {
	return func(value ResolvedCallableQuant) bool { return value != ResolvedCallableA }
}

func resolvedCallableRecursiveLeaf(
	flag bool,
	callback func(ResolvedCallableQuant) bool,
) func(ResolvedCallableQuant) bool {
	if flag {
		return resolvedCallableRecursiveLeaf(false, callback)
	}
	return callback
}
func resolvedCallableDiamondZero() func(ResolvedCallableQuant) bool {
	return resolvedCallableRecursiveLeaf(resolvedCallableFlag, resolvedCallableComplete)
}
func resolvedCallableDiamondOne() func(ResolvedCallableQuant) bool {
	if resolvedCallableFlag {
		return resolvedCallableDiamondZero()
	}
	return resolvedCallableDiamondZero()
}
func resolvedCallableDiamondTwo() func(ResolvedCallableQuant) bool {
	if resolvedCallableFlag {
		return resolvedCallableDiamondOne()
	}
	return resolvedCallableDiamondOne()
}
func resolvedCallableDiamondThree() func(ResolvedCallableQuant) bool {
	if resolvedCallableFlag {
		return resolvedCallableDiamondTwo()
	}
	return resolvedCallableDiamondTwo()
}
func resolvedCallableDiamondFour() func(ResolvedCallableQuant) bool {
	if resolvedCallableFlag {
		return resolvedCallableDiamondThree()
	}
	return resolvedCallableDiamondThree()
}
func resolvedCallableDiamondFive() func(ResolvedCallableQuant) bool {
	if resolvedCallableFlag {
		return resolvedCallableDiamondFour()
	}
	return resolvedCallableDiamondFour()
}
func ResolvedCallableQMatMul(q ResolvedCallableQuant) bool {
	target := resolvedCallableComplete
	set := func(next func(ResolvedCallableQuant) bool) { target = resolvedCallableFunction(next) }
	set(resolvedCallableParameterPartial)
	factory := resolvedCallableFactory
	fromFactory := factory()
	named := resolvedCallableNamedFactory()
	conditional := resolvedCallableConditionalFactory(resolvedCallableConditionalPartial)
	tuple, _ := resolvedCallableTupleFactory()
	generic := resolvedCallableGenericFactory[int]()
	method := resolvedCallableReceiver.factory(
		resolvedCallableReceiver{}, resolvedCallableMethodPartial,
	)
	_, forward := resolvedCallableForwardFactory()
	tupleArgument := resolvedCallableChoose(resolvedCallableTupleArgumentBase())
	literal := resolvedCallableLiteralFactory()
	diamond := resolvedCallableDiamondFive()
	return resolvedCallableComplete(q) && target(q) && fromFactory(q) && named(q) &&
		conditional(q) && tuple(q) && generic(q) && method(q) && forward(q) &&
		tupleArgument(q) && literal(q) && diamond(q)
}

// Package-level function bindings remain ordinary reachable CPU layers.
type PackageFunctionQuant uint8

const (
	PackageFunctionA PackageFunctionQuant = iota // want `quant variant PackageFunctionA \(PackageFunctionQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	PackageFunctionB
	PackageFunctionC
)

func packageFunctionByteSize(q PackageFunctionQuant) int {
	switch q {
	case PackageFunctionA, PackageFunctionB, PackageFunctionC:
		return 8
	default:
		return 0
	}
}
func packageFunctionDecode(q PackageFunctionQuant) []float32 {
	switch q {
	case PackageFunctionA, PackageFunctionB, PackageFunctionC:
		return []float32{}
	default:
		return nil
	}
}

func packageFunctionPartial(q PackageFunctionQuant) bool { return q != PackageFunctionA }
func packageFunctionComplete(q PackageFunctionQuant) bool {
	return q == PackageFunctionA || q == PackageFunctionB || q == PackageFunctionC
}

var packageFunctionLayer = packageFunctionPartial

func PackageFunctionQMatMul(q PackageFunctionQuant) bool {
	return packageFunctionComplete(q) && packageFunctionLayer(q)
}

// Address-taken package bindings become unknown instead of retaining a stale partial target.
type PackageDormantQuant uint8

const (
	PackageDormantA PackageDormantQuant = iota
	PackageDormantB
	PackageDormantC
)

func packageDormantByteSize(q PackageDormantQuant) int {
	switch q {
	case PackageDormantA, PackageDormantB, PackageDormantC:
		return 8
	default:
		return 0
	}
}
func packageDormantDecode(q PackageDormantQuant) []float32 {
	switch q {
	case PackageDormantA, PackageDormantB, PackageDormantC:
		return []float32{}
	default:
		return nil
	}
}

func packageDormantPartial(q PackageDormantQuant) bool { return q != PackageDormantA }
func packageDormantComplete(q PackageDormantQuant) bool {
	return q == PackageDormantA || q == PackageDormantB || q == PackageDormantC
}

var packageDormantLayer = packageDormantPartial

func packageDormantSet(target *func(PackageDormantQuant) bool) {
	*target = packageDormantComplete
}
func init() { packageDormantSet(&packageDormantLayer) }
func PackageDormantQMatMul(q PackageDormantQuant) bool {
	return packageDormantComplete(q) && packageDormantLayer(q)
}

var packageDormantDirectLayer = packageDormantComplete

func packageDormantNeverCalled() { packageDormantDirectLayer = packageDormantPartial }
func PackageDormantDirectQMatMul(q PackageDormantQuant) bool {
	return packageDormantComplete(q) && packageDormantDirectLayer(q)
}

// Anonymous callback aliases retain conditional fixed invocation scope.
type AnonymousAliasQuant uint8

const (
	AnonymousAliasA AnonymousAliasQuant = iota // want `quant variant AnonymousAliasA \(AnonymousAliasQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	AnonymousAliasB
	AnonymousAliasC
)

func anonymousAliasByteSize(q AnonymousAliasQuant) int {
	switch q {
	case AnonymousAliasA, AnonymousAliasB, AnonymousAliasC:
		return 8
	default:
		return 0
	}
}
func anonymousAliasDecode(q AnonymousAliasQuant) []float32 {
	switch q {
	case AnonymousAliasA, AnonymousAliasB, AnonymousAliasC:
		return []float32{}
	default:
		return nil
	}
}

var anonymousAliasEnabled bool

func anonymousAliasPartial(q AnonymousAliasQuant) bool { return q != AnonymousAliasA }
func anonymousAliasComplete(q AnonymousAliasQuant) bool {
	return q == AnonymousAliasA || q == AnonymousAliasB || q == AnonymousAliasC
}
func anonymousAliasDispatch(callback func(AnonymousAliasQuant) bool, _ AnonymousAliasQuant) bool {
	if anonymousAliasEnabled {
		return callback(AnonymousAliasA)
	}
	return true
}
func AnonymousAliasQMatMul(q AnonymousAliasQuant) bool {
	callback := func(value AnonymousAliasQuant) bool { return anonymousAliasPartial(value) }
	return anonymousAliasComplete(q) && anonymousAliasDispatch(callback, q)
}

// Callable receivers and method-expression tuple arguments remain factory inputs.
type MethodFactoryQuant uint8

const (
	MethodFactoryA MethodFactoryQuant = iota // want `quant variant MethodFactoryA \(MethodFactoryQuant\).*absent from 5 of 6 reachable CPU matmul dispatch layers`
	MethodFactoryB
	MethodFactoryC
)

func methodFactoryByteSize(q MethodFactoryQuant) int {
	switch q {
	case MethodFactoryA, MethodFactoryB, MethodFactoryC:
		return 8
	default:
		return 0
	}
}
func methodFactoryDecode(q MethodFactoryQuant) []float32 {
	switch q {
	case MethodFactoryA, MethodFactoryB, MethodFactoryC:
		return []float32{}
	default:
		return nil
	}
}
func methodFactoryPartial(q MethodFactoryQuant) bool { return q != MethodFactoryA }
func methodFactoryTuplePartial(q MethodFactoryQuant) bool {
	return q != MethodFactoryA
}
func methodFactoryReceiverOnePartial(q MethodFactoryQuant) bool {
	return q != MethodFactoryA
}
func methodFactoryReceiverTwoPartial(q MethodFactoryQuant) bool {
	return q != MethodFactoryA
}
func methodFactoryReturnedPartial(q MethodFactoryQuant) bool {
	return q != MethodFactoryA
}
func methodFactoryComplete(q MethodFactoryQuant) bool {
	return q == MethodFactoryA || q == MethodFactoryB || q == MethodFactoryC
}

type methodFactoryFunction func(MethodFactoryQuant) bool

func (function methodFactoryFunction) self() func(MethodFactoryQuant) bool { return function }
func (function methodFactoryFunction) invoke(q MethodFactoryQuant) bool    { return function(q) }

var methodFactoryFlag bool

func methodFactoryReceiverChoice() func(MethodFactoryQuant) bool {
	factory := methodFactoryFunction(methodFactoryReceiverOnePartial).self
	if methodFactoryFlag {
		factory = methodFactoryFunction(methodFactoryReceiverTwoPartial).self
	}
	return factory()
}
func methodFactoryReturned(
	callback func(MethodFactoryQuant) bool,
) func(MethodFactoryQuant) bool {
	return methodFactoryFunction(callback).invoke
}

type methodFactoryReceiver struct{}

func (methodFactoryReceiver) choose(
	_ bool,
	callback func(MethodFactoryQuant) bool,
) func(MethodFactoryQuant) bool {
	return callback
}
func methodFactoryArguments() (
	methodFactoryReceiver,
	bool,
	func(MethodFactoryQuant) bool,
) {
	return methodFactoryReceiver{}, true, methodFactoryTuplePartial
}
func MethodFactoryQMatMul(q MethodFactoryQuant) bool {
	receiver := methodFactoryFunction(methodFactoryPartial).self()
	tuple := methodFactoryReceiver.choose(methodFactoryArguments())
	choice := methodFactoryReceiverChoice()
	returned := methodFactoryReturned(methodFactoryReturnedPartial)
	return methodFactoryComplete(q) && receiver(q) && tuple(q) && choice(q) && returned(q)
}

// Recursive factory contexts propagate callable arguments until the cycle repeats.
type RecursiveFactoryQuant uint8

const (
	RecursiveFactoryA RecursiveFactoryQuant = iota // want `quant variant RecursiveFactoryA \(RecursiveFactoryQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	RecursiveFactoryB
	RecursiveFactoryC
)

func recursiveFactoryByteSize(q RecursiveFactoryQuant) int {
	switch q {
	case RecursiveFactoryA, RecursiveFactoryB, RecursiveFactoryC:
		return 8
	default:
		return 0
	}
}
func recursiveFactoryDecode(q RecursiveFactoryQuant) []float32 {
	switch q {
	case RecursiveFactoryA, RecursiveFactoryB, RecursiveFactoryC:
		return []float32{}
	default:
		return nil
	}
}

var recursiveFactoryFlag bool

func recursiveFactoryPartial(q RecursiveFactoryQuant) bool { return q != RecursiveFactoryA }
func recursiveFactoryComplete(q RecursiveFactoryQuant) bool {
	return q == RecursiveFactoryA || q == RecursiveFactoryB || q == RecursiveFactoryC
}
func recursiveFactory(
	flag bool,
	callback func(RecursiveFactoryQuant) bool,
) func(RecursiveFactoryQuant) bool {
	if flag {
		return recursiveFactory(false, recursiveFactoryPartial)
	}
	return callback
}
func RecursiveFactoryQMatMul(q RecursiveFactoryQuant) bool {
	callback := recursiveFactory(recursiveFactoryFlag, recursiveFactoryComplete)
	return recursiveFactoryComplete(q) && callback(q)
}

// Returned literals preserve captured callable parameters and can themselves be factories.
type CapturedFactoryQuant uint8

const (
	CapturedFactoryA CapturedFactoryQuant = iota // want `quant variant CapturedFactoryA \(CapturedFactoryQuant\).*absent from 7 of 8 reachable CPU matmul dispatch layers`
	CapturedFactoryB
	CapturedFactoryC
)

func capturedFactoryByteSize(q CapturedFactoryQuant) int {
	switch q {
	case CapturedFactoryA, CapturedFactoryB, CapturedFactoryC:
		return 8
	default:
		return 0
	}
}
func capturedFactoryDecode(q CapturedFactoryQuant) []float32 {
	switch q {
	case CapturedFactoryA, CapturedFactoryB, CapturedFactoryC:
		return []float32{}
	default:
		return nil
	}
}
func capturedFactoryPartial(q CapturedFactoryQuant) bool { return q != CapturedFactoryA }
func capturedFactoryLocalPartial(q CapturedFactoryQuant) bool {
	return q != CapturedFactoryA
}
func capturedFactoryLatePartial(q CapturedFactoryQuant) bool {
	return q != CapturedFactoryA
}
func capturedFactoryContextOnePartial(q CapturedFactoryQuant) bool {
	return q != CapturedFactoryA
}
func capturedFactoryContextTwoPartial(q CapturedFactoryQuant) bool {
	return q != CapturedFactoryA
}
func capturedFactoryNestedPartial(q CapturedFactoryQuant) bool {
	return q != CapturedFactoryA
}
func capturedFactoryIIFEPartial(q CapturedFactoryQuant) bool {
	return q != CapturedFactoryA
}
func capturedFactoryComplete(q CapturedFactoryQuant) bool {
	return q == CapturedFactoryA || q == CapturedFactoryB || q == CapturedFactoryC
}

var capturedFactoryFlag bool

func capturedFactoryWrap(
	callback func(CapturedFactoryQuant) bool,
) func(CapturedFactoryQuant) bool {
	alias := callback
	return func(value CapturedFactoryQuant) bool { return alias(value) }
}
func capturedFactoryLateHelper(q CapturedFactoryQuant) bool {
	callback := capturedFactoryWrap(capturedFactoryLatePartial)
	return callback(q)
}
func capturedFactoryHigherWrap(
	callback func(CapturedFactoryQuant) bool,
) func() func(CapturedFactoryQuant) bool {
	return func() func(CapturedFactoryQuant) bool { return callback }
}
func capturedFactoryContexts() func() func(CapturedFactoryQuant) bool {
	var maker func() func(CapturedFactoryQuant) bool
	if capturedFactoryFlag {
		maker = capturedFactoryHigherWrap(capturedFactoryContextOnePartial)
	} else {
		maker = capturedFactoryHigherWrap(capturedFactoryContextTwoPartial)
	}
	return maker
}
func capturedFactoryNestedWrap(
	callback func(CapturedFactoryQuant) bool,
) func() func(CapturedFactoryQuant) bool {
	alias := callback
	return func() func(CapturedFactoryQuant) bool {
		return func(value CapturedFactoryQuant) bool { return alias(value) }
	}
}
func capturedFactoryIIFEWrap(
	callback func(CapturedFactoryQuant) bool,
) func(CapturedFactoryQuant) bool {
	return func(value CapturedFactoryQuant) bool {
		return func() bool { return callback(value) }()
	}
}
func CapturedFactoryQMatMul(q CapturedFactoryQuant) bool {
	captured := capturedFactoryWrap(capturedFactoryPartial)
	maker := func() func(CapturedFactoryQuant) bool { return capturedFactoryLocalPartial }
	local := maker()
	contextMaker := capturedFactoryContexts()
	context := contextMaker()
	nestedMaker := capturedFactoryNestedWrap(capturedFactoryNestedPartial)
	nested := nestedMaker()
	iife := capturedFactoryIIFEWrap(capturedFactoryIIFEPartial)
	return capturedFactoryComplete(q) && captured(q) && local(q) &&
		capturedFactoryLateHelper(q) && context(q) && nested(q) && iife(q)
}

// Deferred rewrites of named callable results invalidate the pre-defer target.
type DeferredFactoryQuant uint8

const (
	DeferredFactoryA DeferredFactoryQuant = iota
	DeferredFactoryB
	DeferredFactoryC
)

func deferredFactoryByteSize(q DeferredFactoryQuant) int {
	switch q {
	case DeferredFactoryA, DeferredFactoryB, DeferredFactoryC:
		return 8
	default:
		return 0
	}
}
func deferredFactoryDecode(q DeferredFactoryQuant) []float32 {
	switch q {
	case DeferredFactoryA, DeferredFactoryB, DeferredFactoryC:
		return []float32{}
	default:
		return nil
	}
}
func deferredFactoryPartial(q DeferredFactoryQuant) bool { return q != DeferredFactoryA }
func deferredFactoryCompleteOne(q DeferredFactoryQuant) bool {
	return q == DeferredFactoryA || q == DeferredFactoryB || q == DeferredFactoryC
}
func deferredFactoryCompleteTwo(q DeferredFactoryQuant) bool {
	return q == DeferredFactoryA || q == DeferredFactoryB || q == DeferredFactoryC
}
func deferredFactory() (result func(DeferredFactoryQuant) bool) {
	result = deferredFactoryPartial
	rewrite := func() { result = deferredFactoryCompleteTwo }
	defer func() { rewrite() }()
	return result
}
func DeferredFactoryQMatMul(q DeferredFactoryQuant) bool {
	callback := deferredFactory()
	return deferredFactoryCompleteOne(q) && deferredFactoryCompleteTwo(q) && callback(q)
}

// A returned literal already invoked in its factory has one layer identity.
type FactoryLiteralIdentityQuant uint8

const (
	FactoryLiteralIdentityA FactoryLiteralIdentityQuant = iota // want `quant variant FactoryLiteralIdentityA \(FactoryLiteralIdentityQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	FactoryLiteralIdentityB
	FactoryLiteralIdentityC
)

func factoryLiteralIdentityByteSize(q FactoryLiteralIdentityQuant) int {
	switch q {
	case FactoryLiteralIdentityA, FactoryLiteralIdentityB, FactoryLiteralIdentityC:
		return 8
	default:
		return 0
	}
}
func factoryLiteralIdentityDecode(q FactoryLiteralIdentityQuant) []float32 {
	switch q {
	case FactoryLiteralIdentityA, FactoryLiteralIdentityB, FactoryLiteralIdentityC:
		return []float32{}
	default:
		return nil
	}
}

var factoryLiteralIdentityFlag bool

func factoryLiteralIdentityComplete(q FactoryLiteralIdentityQuant) bool {
	return q == FactoryLiteralIdentityA || q == FactoryLiteralIdentityB || q == FactoryLiteralIdentityC
}
func factoryLiteralIdentity(
	_ FactoryLiteralIdentityQuant,
) func(FactoryLiteralIdentityQuant) bool {
	literal := func(value FactoryLiteralIdentityQuant) bool {
		return value != FactoryLiteralIdentityA
	}
	if factoryLiteralIdentityFlag {
		_ = literal(FactoryLiteralIdentityA)
	}
	return literal
}
func FactoryLiteralIdentityQMatMul(q FactoryLiteralIdentityQuant) bool {
	callback := factoryLiteralIdentity(q)
	return factoryLiteralIdentityComplete(q) && callback(q)
}

// Assignments inside a returned closure replace its captured entry binding.
type CaptureRewriteQuant uint8

const (
	CaptureRewriteA CaptureRewriteQuant = iota
	CaptureRewriteB
	CaptureRewriteC
)

func captureRewriteByteSize(q CaptureRewriteQuant) int {
	switch q {
	case CaptureRewriteA, CaptureRewriteB, CaptureRewriteC:
		return 8
	default:
		return 0
	}
}
func captureRewriteDecode(q CaptureRewriteQuant) []float32 {
	switch q {
	case CaptureRewriteA, CaptureRewriteB, CaptureRewriteC:
		return []float32{}
	default:
		return nil
	}
}
func captureRewritePartial(q CaptureRewriteQuant) bool { return q != CaptureRewriteA }
func captureRewriteCompleteOne(q CaptureRewriteQuant) bool {
	return q == CaptureRewriteA || q == CaptureRewriteB || q == CaptureRewriteC
}
func captureRewriteCompleteTwo(q CaptureRewriteQuant) bool {
	return q == CaptureRewriteA || q == CaptureRewriteB || q == CaptureRewriteC
}
func captureRewriteWrap(
	callback func(CaptureRewriteQuant) bool,
) func(CaptureRewriteQuant) bool {
	return func(value CaptureRewriteQuant) bool {
		callback = captureRewriteCompleteTwo
		return callback(value)
	}
}
func captureRewriteAtCreation(
	callback func(CaptureRewriteQuant) bool,
) func(CaptureRewriteQuant) bool {
	callback = captureRewriteCompleteTwo
	return func(value CaptureRewriteQuant) bool { return callback(value) }
}

type captureRewriteRunner struct {
	callback func(CaptureRewriteQuant) bool
}

func (runner *captureRewriteRunner) invoke(value CaptureRewriteQuant) bool {
	return runner.callback(value)
}
func captureRewritePointerMethod(
	callback func(CaptureRewriteQuant) bool,
) func(CaptureRewriteQuant) bool {
	runner := &captureRewriteRunner{callback: callback}
	method := runner.invoke
	runner.callback = captureRewriteCompleteTwo
	return method
}
func captureRewritePointerAlias(
	callback func(CaptureRewriteQuant) bool,
) func(CaptureRewriteQuant) bool {
	runner := &captureRewriteRunner{callback: callback}
	alias := runner
	method := runner.invoke
	alias.callback = captureRewriteCompleteTwo
	return method
}
func captureRewriteCorrelated(
	callback func(CaptureRewriteQuant) bool,
	flag bool,
) func(CaptureRewriteQuant) bool {
	return func(value CaptureRewriteQuant) bool {
		if flag {
			callback = captureRewriteCompleteTwo
		}
		if flag {
			return func() bool { return callback(value) }()
		}
		return captureRewriteCompleteTwo(value)
	}
}
func CaptureRewriteQMatMul(q CaptureRewriteQuant) bool {
	callback := captureRewriteWrap(captureRewritePartial)
	created := captureRewriteAtCreation(captureRewritePartial)
	mutated := captureRewritePointerWrap(captureRewriteFunction(captureRewritePartial))
	pointerMethod := captureRewritePointerMethod(captureRewritePartial)
	pointerAlias := captureRewritePointerAlias(captureRewritePartial)
	correlated := captureRewriteCorrelated(captureRewritePartial, captureRewriteFlag)
	return captureRewriteCompleteOne(q) && captureRewriteCompleteTwo(q) &&
		callback(q) && created(q) && mutated(q) && pointerMethod(q) && pointerAlias(q) && correlated(q)
}

var captureRewriteFlag bool

type captureRewriteFunction func(CaptureRewriteQuant) bool

func (function *captureRewriteFunction) setComplete() {
	*function = captureRewriteCompleteTwo
}
func captureRewritePointerWrap(
	callback captureRewriteFunction,
) func(CaptureRewriteQuant) bool {
	callback.setComplete()
	return func(value CaptureRewriteQuant) bool { return callback(value) }
}

// Callable self-assignments preserve entry targets while nested IIFE writes replace them in order.
type NestedCaptureQuant uint8

const (
	NestedCaptureA NestedCaptureQuant = iota // want `quant variant NestedCaptureA \(NestedCaptureQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	NestedCaptureB
	NestedCaptureC
)

func nestedCaptureByteSize(q NestedCaptureQuant) int {
	switch q {
	case NestedCaptureA, NestedCaptureB, NestedCaptureC:
		return 8
	default:
		return 0
	}
}
func nestedCaptureDecode(q NestedCaptureQuant) []float32 {
	switch q {
	case NestedCaptureA, NestedCaptureB, NestedCaptureC:
		return []float32{}
	default:
		return nil
	}
}
func nestedCaptureDiscardedPartial(q NestedCaptureQuant) bool { return q != NestedCaptureA }
func nestedCaptureReversePartial(q NestedCaptureQuant) bool   { return q != NestedCaptureA }
func nestedCaptureSelfPartial(q NestedCaptureQuant) bool      { return q != NestedCaptureA }
func nestedCaptureComplete(q NestedCaptureQuant) bool {
	return q == NestedCaptureA || q == NestedCaptureB || q == NestedCaptureC
}
func nestedCaptureRewriteBefore(
	callback func(NestedCaptureQuant) bool,
) func(NestedCaptureQuant) bool {
	return func(value NestedCaptureQuant) bool {
		return func() bool {
			callback = nestedCaptureComplete
			return func() bool { return callback(value) }()
		}()
	}
}
func nestedCaptureRewriteAfter(
	callback func(NestedCaptureQuant) bool,
) func(NestedCaptureQuant) bool {
	return func(value NestedCaptureQuant) bool {
		return func() bool {
			result := func() bool { return callback(value) }()
			callback = nestedCaptureComplete
			return result
		}()
	}
}
func nestedCaptureSelf(
	callback func(NestedCaptureQuant) bool,
) func(NestedCaptureQuant) bool {
	return func(value NestedCaptureQuant) bool {
		callback = callback
		return callback(value)
	}
}
func NestedCaptureQMatMul(q NestedCaptureQuant) bool {
	discarded := nestedCaptureRewriteBefore(nestedCaptureDiscardedPartial)
	reverse := nestedCaptureRewriteAfter(nestedCaptureReversePartial)
	self := nestedCaptureSelf(nestedCaptureSelfPartial)
	return nestedCaptureComplete(q) && discarded(q) && reverse(q) && self(q)
}

// Factory-returned methods project callable holder fields and pointer-to-function receivers.
type ReceiverProjectionQuant uint8

const (
	ReceiverProjectionA ReceiverProjectionQuant = iota // want `quant variant ReceiverProjectionA \(ReceiverProjectionQuant\).*absent from 22 of 23 reachable CPU matmul dispatch layers`
	ReceiverProjectionB
	ReceiverProjectionC
)

func receiverProjectionByteSize(q ReceiverProjectionQuant) int {
	switch q {
	case ReceiverProjectionA, ReceiverProjectionB, ReceiverProjectionC:
		return 8
	default:
		return 0
	}
}
func receiverProjectionDecode(q ReceiverProjectionQuant) []float32 {
	switch q {
	case ReceiverProjectionA, ReceiverProjectionB, ReceiverProjectionC:
		return []float32{}
	default:
		return nil
	}
}
func receiverProjectionHolderPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionPointerPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionNestedPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionRepeatedPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionDormantPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionPromotedPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionPrewritePartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionDeadPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionAliasPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionSetterPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionBranchPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionSetterAliasPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionStalePartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionExhaustiveSetterPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionExhaustiveOuterPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionConditionalRootPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionExhaustiveRootPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionNestedSetterPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionRHSSetterPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionNamedSetterPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionNamedAliasSetterPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionNamedLocalSetterPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionNamedReboundControlPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionNamedDeadRebindPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionNamedTerminatingRebindPartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionNamedExhaustivePartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionNamedRestorePartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionTuplePartial(q ReceiverProjectionQuant) bool {
	return q != ReceiverProjectionA
}
func receiverProjectionComplete(q ReceiverProjectionQuant) bool {
	return q == ReceiverProjectionA || q == ReceiverProjectionB || q == ReceiverProjectionC
}

type receiverProjectionRunner struct {
	callback func(ReceiverProjectionQuant) bool
}

func (runner receiverProjectionRunner) invoke(q ReceiverProjectionQuant) bool {
	return runner.callback(q)
}
func receiverProjectionHolderFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	return receiverProjectionRunner{callback: callback}.invoke
}

type receiverProjectionFunction func(ReceiverProjectionQuant) bool

func (function *receiverProjectionFunction) invoke(q ReceiverProjectionQuant) bool {
	return (*function)(q)
}
func receiverProjectionPointerFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	function := receiverProjectionFunction(callback)
	return (&function).invoke
}

type receiverProjectionConfig struct {
	callback func(ReceiverProjectionQuant) bool
}

type receiverProjectionNestedRunner struct {
	config receiverProjectionConfig
}

func (runner receiverProjectionNestedRunner) invoke(q ReceiverProjectionQuant) bool {
	return runner.config.callback(q)
}
func receiverProjectionNestedFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	return receiverProjectionNestedRunner{
		config: receiverProjectionConfig{callback: callback},
	}.invoke
}

type receiverProjectionRepeatedRunner struct {
	left  receiverProjectionConfig
	right receiverProjectionConfig
}

func (runner receiverProjectionRepeatedRunner) invoke(q ReceiverProjectionQuant) bool {
	return runner.left.callback(q)
}
func receiverProjectionRepeatedFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	return receiverProjectionRepeatedRunner{
		left:  receiverProjectionConfig{callback: callback},
		right: receiverProjectionConfig{callback: receiverProjectionComplete},
	}.invoke
}

type receiverProjectionPointerRunner struct {
	callback func(ReceiverProjectionQuant) bool
}

func (runner *receiverProjectionPointerRunner) invoke(q ReceiverProjectionQuant) bool {
	return runner.callback(q)
}
func receiverProjectionDormantFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: callback}
	method := runner.invoke
	_ = func() { runner.callback = receiverProjectionComplete }
	return method
}

type receiverProjectionPromotedRunner struct {
	receiverProjectionConfig
}

func (runner receiverProjectionPromotedRunner) invoke(q ReceiverProjectionQuant) bool {
	return runner.callback(q)
}
func receiverProjectionPromotedFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	return receiverProjectionPromotedRunner{
		receiverProjectionConfig: receiverProjectionConfig{callback: callback},
	}.invoke
}
func receiverProjectionPrewriteFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	runner.callback = callback
	return runner.invoke
}
func receiverProjectionDeadFactory() func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	if false {
		runner.callback = receiverProjectionDeadPartial
	}
	return runner.invoke
}
func receiverProjectionAliasFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	alias := runner
	alias.callback = callback
	return runner.invoke
}
func receiverProjectionSetterFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	set := func() { runner.callback = callback }
	runner.callback = receiverProjectionComplete
	set()
	return runner.invoke
}
func receiverProjectionBranchFactory(
	callback func(ReceiverProjectionQuant) bool,
	flag bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	runner.callback = receiverProjectionComplete
	if flag {
		runner.callback = callback
	} else {
		runner.callback = callback
	}
	return runner.invoke
}
func receiverProjectionSetterAliasFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	set := func() {
		alias := runner
		other := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
		if receiverProjectionBranchFlag {
			alias = other
		}
		alias.callback = callback
	}
	set()
	return runner.invoke
}
func receiverProjectionStaleFactory() func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	stale := runner
	runner = &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	stale.callback = receiverProjectionStalePartial
	return runner.invoke
}
func receiverProjectionExhaustiveSetterFactory() func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	set := func() {
		alias := runner
		other := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
		third := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
		if receiverProjectionBranchFlag {
			alias = other
		} else {
			alias = third
		}
		alias.callback = receiverProjectionExhaustiveSetterPartial
	}
	set()
	return runner.invoke
}
func receiverProjectionExhaustiveOuterFactory() func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	alias := runner
	other := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	third := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	if receiverProjectionBranchFlag {
		alias = other
	} else {
		alias = third
	}
	alias.callback = receiverProjectionExhaustiveOuterPartial
	return runner.invoke
}
func receiverProjectionConditionalRootFactory() func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	stale := runner
	if receiverProjectionBranchFlag {
		runner = &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	}
	stale.callback = receiverProjectionConditionalRootPartial
	return runner.invoke
}
func receiverProjectionExhaustiveRootFactory() func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	stale := runner
	if receiverProjectionBranchFlag {
		runner = &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	} else {
		runner = &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	}
	stale.callback = receiverProjectionExhaustiveRootPartial
	return runner.invoke
}
func receiverProjectionNestedSetterFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	set := func() {
		alias := runner
		inner := func() { alias.callback = callback }
		inner()
		alias = &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	}
	runner.callback = receiverProjectionComplete
	set()
	return runner.invoke
}
func receiverProjectionRHSSetterFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	set := func() {
		alias := runner
		alias = func() *receiverProjectionPointerRunner {
			inner := func() { alias.callback = callback }
			inner()
			return &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
		}()
	}
	set()
	return runner.invoke
}
func receiverProjectionNamedSetter(
	runner *receiverProjectionPointerRunner,
	callback func(ReceiverProjectionQuant) bool,
) {
	runner.callback = callback
}
func receiverProjectionNamedSetterFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	runner.callback = receiverProjectionComplete
	receiverProjectionNamedSetter(runner, callback)
	return runner.invoke
}
func receiverProjectionNamedAliasSetterFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	setter := receiverProjectionNamedSetter
	setter(runner, callback)
	return runner.invoke
}
func receiverProjectionNamedLocalSetter(
	runner *receiverProjectionPointerRunner,
	callback func(ReceiverProjectionQuant) bool,
) {
	local := callback
	runner.callback = local
}
func receiverProjectionNamedLocalSetterFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	setter := receiverProjectionNamedLocalSetter
	setter(runner, callback)
	return runner.invoke
}
func receiverProjectionNamedReboundSetter(
	runner *receiverProjectionPointerRunner,
	callback func(ReceiverProjectionQuant) bool,
) {
	callback = receiverProjectionComplete
	local := callback
	runner.callback = local
}
func receiverProjectionNamedReboundFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	setter := receiverProjectionNamedReboundSetter
	setter(runner, callback)
	return runner.invoke
}
func receiverProjectionNamedDeadRebindSetter(
	runner *receiverProjectionPointerRunner,
	callback func(ReceiverProjectionQuant) bool,
) {
	if false {
		callback = receiverProjectionComplete
	}
	local := callback
	runner.callback = local
}
func receiverProjectionNamedDeadRebindFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	setter := receiverProjectionNamedDeadRebindSetter
	setter(runner, callback)
	return runner.invoke
}
func receiverProjectionNamedTerminatingRebindSetter(
	runner *receiverProjectionPointerRunner,
	callback func(ReceiverProjectionQuant) bool,
	flag bool,
) {
	if flag {
		callback = receiverProjectionComplete
		return
	}
	local := callback
	runner.callback = local
}
func receiverProjectionNamedTerminatingRebindFactory(
	callback func(ReceiverProjectionQuant) bool,
	flag bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	setter := receiverProjectionNamedTerminatingRebindSetter
	setter(runner, callback, flag)
	return runner.invoke
}
func receiverProjectionNamedExhaustiveSetter(
	runner *receiverProjectionPointerRunner,
	callback func(ReceiverProjectionQuant) bool,
	flag bool,
) {
	if flag {
		runner.callback = callback
	} else {
		runner.callback = receiverProjectionComplete
	}
}
func receiverProjectionNamedExhaustiveFactory(
	callback func(ReceiverProjectionQuant) bool,
	flag bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	setter := receiverProjectionNamedExhaustiveSetter
	setter(runner, callback, flag)
	return runner.invoke
}
func receiverProjectionNamedRestoreSetter(
	runner *receiverProjectionPointerRunner,
	callback func(ReceiverProjectionQuant) bool,
) {
	_ = fmt.Sprint(runner)
	runner.callback = callback
}
func receiverProjectionNamedRestoreFactory(
	callback func(ReceiverProjectionQuant) bool,
) func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	setter := receiverProjectionNamedRestoreSetter
	setter(runner, callback)
	return runner.invoke
}
func receiverProjectionTuplePair() (func(ReceiverProjectionQuant) bool, bool) {
	return receiverProjectionTuplePartial, true
}
func receiverProjectionTupleFactory() func(ReceiverProjectionQuant) bool {
	runner := &receiverProjectionPointerRunner{callback: receiverProjectionComplete}
	runner.callback, _ = receiverProjectionTuplePair()
	return runner.invoke
}
func ReceiverProjectionQMatMul(q ReceiverProjectionQuant) bool {
	holder := receiverProjectionHolderFactory(receiverProjectionHolderPartial)
	pointer := receiverProjectionPointerFactory(receiverProjectionPointerPartial)
	nested := receiverProjectionNestedFactory(receiverProjectionNestedPartial)
	repeated := receiverProjectionRepeatedFactory(receiverProjectionRepeatedPartial)
	dormant := receiverProjectionDormantFactory(receiverProjectionDormantPartial)
	promoted := receiverProjectionPromotedFactory(receiverProjectionPromotedPartial)
	prewrite := receiverProjectionPrewriteFactory(receiverProjectionPrewritePartial)
	dead := receiverProjectionDeadFactory()
	alias := receiverProjectionAliasFactory(receiverProjectionAliasPartial)
	setter := receiverProjectionSetterFactory(receiverProjectionSetterPartial)
	branch := receiverProjectionBranchFactory(
		receiverProjectionBranchPartial, receiverProjectionBranchFlag,
	)
	setterAlias := receiverProjectionSetterAliasFactory(receiverProjectionSetterAliasPartial)
	stale := receiverProjectionStaleFactory()
	exhaustiveSetter := receiverProjectionExhaustiveSetterFactory()
	exhaustiveOuter := receiverProjectionExhaustiveOuterFactory()
	conditionalRoot := receiverProjectionConditionalRootFactory()
	exhaustiveRoot := receiverProjectionExhaustiveRootFactory()
	nestedSetter := receiverProjectionNestedSetterFactory(receiverProjectionNestedSetterPartial)
	rhsSetter := receiverProjectionRHSSetterFactory(receiverProjectionRHSSetterPartial)
	namedSetter := receiverProjectionNamedSetterFactory(receiverProjectionNamedSetterPartial)
	namedAliasSetter := receiverProjectionNamedAliasSetterFactory(
		receiverProjectionNamedAliasSetterPartial,
	)
	namedLocalSetter := receiverProjectionNamedLocalSetterFactory(
		receiverProjectionNamedLocalSetterPartial,
	)
	namedRebound := receiverProjectionNamedReboundFactory(
		receiverProjectionNamedReboundControlPartial,
	)
	namedDeadRebind := receiverProjectionNamedDeadRebindFactory(
		receiverProjectionNamedDeadRebindPartial,
	)
	namedTerminatingRebind := receiverProjectionNamedTerminatingRebindFactory(
		receiverProjectionNamedTerminatingRebindPartial, receiverProjectionBranchFlag,
	)
	namedExhaustive := receiverProjectionNamedExhaustiveFactory(
		receiverProjectionNamedExhaustivePartial, receiverProjectionBranchFlag,
	)
	namedRestore := receiverProjectionNamedRestoreFactory(receiverProjectionNamedRestorePartial)
	tuple := receiverProjectionTupleFactory()
	return receiverProjectionComplete(q) && holder(q) && pointer(q) && nested(q) &&
		repeated(q) && dormant(q) && promoted(q) && prewrite(q) && dead(q) && alias(q) &&
		setter(q) && branch(q) && setterAlias(q) && stale(q) && exhaustiveSetter(q) &&
		exhaustiveOuter(q) && conditionalRoot(q) && exhaustiveRoot(q) && nestedSetter(q) &&
		rhsSetter(q) && namedSetter(q) && namedAliasSetter(q) && namedLocalSetter(q) &&
		namedRebound(q) && namedDeadRebind(q) && namedTerminatingRebind(q) &&
		namedExhaustive(q) && namedRestore(q) && tuple(q)
}

var receiverProjectionBranchFlag bool

// Internal and returned invocations union their exact literal argument scopes.
type ReturnedScopeQuant uint8

const (
	ReturnedScopeA ReturnedScopeQuant = iota
	ReturnedScopeB                    // want `quant variant ReturnedScopeB \(ReturnedScopeQuant\).*absent from 1 of 3 reachable CPU matmul dispatch layers`
	ReturnedScopeC
)

func returnedScopeByteSize(q ReturnedScopeQuant) int {
	switch q {
	case ReturnedScopeA, ReturnedScopeB, ReturnedScopeC:
		return 8
	default:
		return 0
	}
}
func returnedScopeDecode(q ReturnedScopeQuant) []float32 {
	switch q {
	case ReturnedScopeA, ReturnedScopeB, ReturnedScopeC:
		return []float32{}
	default:
		return nil
	}
}
func returnedScopeCompleteOne(q ReturnedScopeQuant) bool {
	return q == ReturnedScopeA || q == ReturnedScopeB || q == ReturnedScopeC
}
func returnedScopeCompleteTwo(q ReturnedScopeQuant) bool {
	return q == ReturnedScopeA || q == ReturnedScopeB || q == ReturnedScopeC
}
func returnedScopePartialHelper(value ReturnedScopeQuant) bool {
	return value != ReturnedScopeB
}
func returnedScopeFactory() func(ReturnedScopeQuant) bool {
	literal := func(value ReturnedScopeQuant) bool {
		alias := value
		return func() bool { return returnedScopePartialHelper(alias) }()
	}
	_ = literal(ReturnedScopeA)
	return literal
}
func returnedScopeDispatch(
	callback func(ReturnedScopeQuant) bool,
	value ReturnedScopeQuant,
) bool {
	return callback(value)
}
func ReturnedScopeQMatMul(q ReturnedScopeQuant) bool {
	callback := returnedScopeFactory()
	return returnedScopeCompleteOne(q) && returnedScopeCompleteTwo(q) &&
		returnedScopeDispatch(callback, ReturnedScopeB)
}

// Finite scopes from distinct returned-literal subjects stay finite when merged.
type FiniteReturnedScopeQuant uint8

const (
	FiniteReturnedScopeA FiniteReturnedScopeQuant = iota // want `quant variant FiniteReturnedScopeA \(FiniteReturnedScopeQuant\).*absent from 2 of 5 reachable CPU matmul dispatch layers`
	FiniteReturnedScopeB                                 // want `quant variant FiniteReturnedScopeB \(FiniteReturnedScopeQuant\).*absent from 1 of 5 reachable CPU matmul dispatch layers`
	FiniteReturnedScopeC                                 // want `quant variant FiniteReturnedScopeC \(FiniteReturnedScopeQuant\).*absent from 1 of 5 reachable CPU matmul dispatch layers`
	FiniteReturnedScopeD                                 // want `quant variant FiniteReturnedScopeD \(FiniteReturnedScopeQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
)

func finiteReturnedScopeByteSize(q FiniteReturnedScopeQuant) int {
	switch q {
	case FiniteReturnedScopeA, FiniteReturnedScopeB, FiniteReturnedScopeC, FiniteReturnedScopeD:
		return 8
	default:
		return 0
	}
}
func finiteReturnedScopeDecode(q FiniteReturnedScopeQuant) []float32 {
	switch q {
	case FiniteReturnedScopeA, FiniteReturnedScopeB, FiniteReturnedScopeC, FiniteReturnedScopeD:
		return []float32{}
	default:
		return nil
	}
}
func finiteReturnedScopePartial(value FiniteReturnedScopeQuant) bool {
	return value != FiniteReturnedScopeD
}
func finiteReturnedScopeCompleteOne(value FiniteReturnedScopeQuant) bool {
	return value == FiniteReturnedScopeA || value == FiniteReturnedScopeB ||
		value == FiniteReturnedScopeC || value == FiniteReturnedScopeD
}
func finiteReturnedScopeCompleteTwo(value FiniteReturnedScopeQuant) bool {
	return value == FiniteReturnedScopeA || value == FiniteReturnedScopeB ||
		value == FiniteReturnedScopeC || value == FiniteReturnedScopeD
}
func finiteReturnedScopeFactory() func(FiniteReturnedScopeQuant) bool {
	literal := func(value FiniteReturnedScopeQuant) bool { return finiteReturnedScopePartial(value) }
	_ = literal(FiniteReturnedScopeA)
	return literal
}
func FiniteReturnedScopeQMatMul(
	left FiniteReturnedScopeQuant,
	right FiniteReturnedScopeQuant,
) bool {
	callback := finiteReturnedScopeFactory()
	result := finiteReturnedScopeCompleteOne(left) && finiteReturnedScopeCompleteTwo(right)
	if left == FiniteReturnedScopeB {
		result = result && callback(left)
	}
	if right == FiniteReturnedScopeC {
		result = result && callback(right)
	}
	return result
}

// Direct support sites and global maps inside returned literals inherit every
// enclosing literal's external fixed-value scope, including local aliases.
type DirectReturnedScopeQuant uint8

const (
	DirectReturnedScopeA DirectReturnedScopeQuant = iota
	DirectReturnedScopeB                          // want `quant variant DirectReturnedScopeB \(DirectReturnedScopeQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	DirectReturnedScopeC
)

func directReturnedScopeByteSize(q DirectReturnedScopeQuant) int {
	switch q {
	case DirectReturnedScopeA, DirectReturnedScopeB, DirectReturnedScopeC:
		return 8
	default:
		return 0
	}
}
func directReturnedScopeDecode(q DirectReturnedScopeQuant) []float32 {
	switch q {
	case DirectReturnedScopeA, DirectReturnedScopeB, DirectReturnedScopeC:
		return []float32{}
	default:
		return nil
	}
}
func directReturnedScopeCompleteOne(q DirectReturnedScopeQuant) bool {
	return q == DirectReturnedScopeA || q == DirectReturnedScopeB || q == DirectReturnedScopeC
}
func directReturnedScopeCompleteTwo(q DirectReturnedScopeQuant) bool {
	return q == DirectReturnedScopeA || q == DirectReturnedScopeB || q == DirectReturnedScopeC
}

var directReturnedScopeDispatch = map[DirectReturnedScopeQuant]func(){
	DirectReturnedScopeA: func() {},
	DirectReturnedScopeC: func() {},
}

func directReturnedScopeFactory() func(DirectReturnedScopeQuant) bool {
	literal := func(value DirectReturnedScopeQuant) bool {
		alias := value
		return func() bool {
			return alias != DirectReturnedScopeB && directReturnedScopeDispatch[alias] != nil
		}()
	}
	_ = literal(DirectReturnedScopeA)
	return literal
}
func DirectReturnedScopeQMatMul(q DirectReturnedScopeQuant) bool {
	callback := directReturnedScopeFactory()
	return directReturnedScopeCompleteOne(q) && directReturnedScopeCompleteTwo(q) &&
		callback(DirectReturnedScopeB)
}

// A factory-returned enum method value keeps the receiver scope from the call
// which created it; an incomplete method fixed to B says nothing about A.
type ReceiverScopeQuant uint8

const (
	ReceiverScopeA ReceiverScopeQuant = iota
	ReceiverScopeB
	ReceiverScopeC
)

func receiverScopeByteSize(q ReceiverScopeQuant) int {
	switch q {
	case ReceiverScopeA, ReceiverScopeB, ReceiverScopeC:
		return 8
	default:
		return 0
	}
}
func receiverScopeDecode(q ReceiverScopeQuant) []float32 {
	switch q {
	case ReceiverScopeA, ReceiverScopeB, ReceiverScopeC:
		return []float32{}
	default:
		return nil
	}
}
func receiverScopeComplete(q ReceiverScopeQuant) bool {
	return q == ReceiverScopeA || q == ReceiverScopeB || q == ReceiverScopeC
}
func (q ReceiverScopeQuant) partial() bool { return q != ReceiverScopeA }
func receiverScopeFactory(q ReceiverScopeQuant) func() bool {
	return q.partial
}
func (q ReceiverScopeQuant) factory() func() bool {
	return q.partial
}
func ReceiverScopeQMatMul(q ReceiverScopeQuant) bool {
	return receiverScopeComplete(q) && receiverScopeFactory(ReceiverScopeB)() &&
		ReceiverScopeB.factory()()
}

// Fixed non-callable factory arguments prune unreachable returned callbacks.
type FactoryArgumentQuant uint8

const (
	FactoryArgumentA FactoryArgumentQuant = iota
	FactoryArgumentB
	FactoryArgumentC
)

func factoryArgumentByteSize(q FactoryArgumentQuant) int {
	switch q {
	case FactoryArgumentA, FactoryArgumentB, FactoryArgumentC:
		return 8
	default:
		return 0
	}
}
func factoryArgumentDecode(q FactoryArgumentQuant) []float32 {
	switch q {
	case FactoryArgumentA, FactoryArgumentB, FactoryArgumentC:
		return []float32{}
	default:
		return nil
	}
}
func factoryArgumentPartial(q FactoryArgumentQuant) bool { return q != FactoryArgumentA }
func factoryArgumentComplete(q FactoryArgumentQuant) bool {
	return q == FactoryArgumentA || q == FactoryArgumentB || q == FactoryArgumentC
}
func factoryArgumentCompleteTwo(q FactoryArgumentQuant) bool {
	return q == FactoryArgumentA || q == FactoryArgumentB || q == FactoryArgumentC
}
func factoryArgumentFactory(
	which FactoryArgumentQuant,
) func(FactoryArgumentQuant) bool {
	if which == FactoryArgumentB {
		return factoryArgumentComplete
	}
	return factoryArgumentPartial
}
func factoryArgumentSwitchFactory(
	which FactoryArgumentQuant,
) func(FactoryArgumentQuant) bool {
	switch which {
	case FactoryArgumentB:
		return factoryArgumentComplete
	default:
		return factoryArgumentPartial
	}
}
func factoryArgumentSwitchContinuationFactory(
	which FactoryArgumentQuant,
) func(FactoryArgumentQuant) bool {
	switch which {
	case FactoryArgumentB:
		return factoryArgumentComplete
	}
	return factoryArgumentPartial
}
func factoryArgumentAddressFactory(
	which FactoryArgumentQuant,
) func(FactoryArgumentQuant) bool {
	if &which != nil {
		return factoryArgumentComplete
	}
	return factoryArgumentCompleteTwo
}
func factoryArgumentDivisionFactory(
	which FactoryArgumentQuant,
) func(FactoryArgumentQuant) bool {
	if 1/(which-which) == 0 {
		return factoryArgumentComplete
	}
	return factoryArgumentCompleteTwo
}
func FactoryArgumentQMatMul(q FactoryArgumentQuant) bool {
	callback := factoryArgumentFactory(FactoryArgumentB)
	switched := factoryArgumentSwitchFactory(FactoryArgumentB)
	continued := factoryArgumentSwitchContinuationFactory(FactoryArgumentB)
	addressed := factoryArgumentAddressFactory(FactoryArgumentB)
	division := factoryArgumentDivisionFactory(FactoryArgumentB)
	return factoryArgumentComplete(q) && callback(q) && switched(q) && continued(q) &&
		addressed(q) && division(q)
}

// A fixed factory argument stops being fixed after a local reassignment.
type FactoryMutationQuant uint8

const (
	FactoryMutationA FactoryMutationQuant = iota // want `quant variant FactoryMutationA \(FactoryMutationQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	FactoryMutationB
	FactoryMutationC
)

func factoryMutationByteSize(q FactoryMutationQuant) int {
	switch q {
	case FactoryMutationA, FactoryMutationB, FactoryMutationC:
		return 8
	default:
		return 0
	}
}
func factoryMutationDecode(q FactoryMutationQuant) []float32 {
	switch q {
	case FactoryMutationA, FactoryMutationB, FactoryMutationC:
		return []float32{}
	default:
		return nil
	}
}
func factoryMutationPartial(q FactoryMutationQuant) bool { return q != FactoryMutationA }
func factoryMutationIIFEPartial(q FactoryMutationQuant) bool {
	return q != FactoryMutationA
}
func factoryMutationLatePartial(q FactoryMutationQuant) bool {
	return q != FactoryMutationA
}
func factoryMutationPathPartial(q FactoryMutationQuant) bool {
	return q != FactoryMutationA
}
func factoryMutationComplete(q FactoryMutationQuant) bool {
	return q == FactoryMutationA || q == FactoryMutationB || q == FactoryMutationC
}
func factoryMutationFactory(
	which FactoryMutationQuant,
) func(FactoryMutationQuant) bool {
	which = FactoryMutationA
	if which == FactoryMutationB {
		return factoryMutationComplete
	}
	return factoryMutationPartial
}
func factoryMutationIIFEFactory(
	which FactoryMutationQuant,
) func(FactoryMutationQuant) bool {
	func() { which = FactoryMutationA }()
	if which == FactoryMutationB {
		return factoryMutationComplete
	}
	return factoryMutationIIFEPartial
}
func factoryMutationLateSetterFactory(
	which FactoryMutationQuant,
) func(FactoryMutationQuant) bool {
	set := func() { which = FactoryMutationA }
	if which == FactoryMutationB {
		return factoryMutationComplete
	}
	set()
	return factoryMutationLatePartial
}
func factoryMutationPathFactory(
	which FactoryMutationQuant,
	flag bool,
) func(FactoryMutationQuant) bool {
	set := func() { which = FactoryMutationA }
	if flag {
		set()
		return factoryMutationComplete
	}
	if which == FactoryMutationB {
		return factoryMutationComplete
	}
	return factoryMutationPathPartial
}
func FactoryMutationQMatMul(q FactoryMutationQuant) bool {
	callback := factoryMutationFactory(FactoryMutationB)
	iife := factoryMutationIIFEFactory(FactoryMutationB)
	late := factoryMutationLateSetterFactory(FactoryMutationB)
	path := factoryMutationPathFactory(FactoryMutationB, factoryMutationPathFlag)
	return factoryMutationComplete(q) && callback(q) && iife(q) && late(q) && path(q)
}

var factoryMutationPathFlag bool

// Repeated guards are correlated only while their predicate object cannot be
// mutated through an alias between them.
type CorrelatedAliasQuant uint8

const (
	CorrelatedAliasA CorrelatedAliasQuant = iota // want `quant variant CorrelatedAliasA \(CorrelatedAliasQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	CorrelatedAliasB
	CorrelatedAliasC
)

func correlatedAliasByteSize(q CorrelatedAliasQuant) int {
	switch q {
	case CorrelatedAliasA, CorrelatedAliasB, CorrelatedAliasC:
		return 8
	default:
		return 0
	}
}
func correlatedAliasDecode(q CorrelatedAliasQuant) []float32 {
	switch q {
	case CorrelatedAliasA, CorrelatedAliasB, CorrelatedAliasC:
		return []float32{}
	default:
		return nil
	}
}
func correlatedAliasPartial(q CorrelatedAliasQuant) bool { return q != CorrelatedAliasA }
func correlatedCapturePartial(q CorrelatedAliasQuant) bool {
	return q != CorrelatedAliasA
}
func correlatedDormantPartial(q CorrelatedAliasQuant) bool {
	return q != CorrelatedAliasA
}
func correlatedAliasComplete(q CorrelatedAliasQuant) bool {
	return q == CorrelatedAliasA || q == CorrelatedAliasB || q == CorrelatedAliasC
}
func correlatedAliasFactory(
	callback func(CorrelatedAliasQuant) bool,
	flag bool,
) func(CorrelatedAliasQuant) bool {
	pointer := &flag
	return func(value CorrelatedAliasQuant) bool {
		if flag {
			callback = correlatedAliasComplete
		}
		*pointer = !flag
		if flag {
			return callback(value)
		}
		return correlatedAliasComplete(value)
	}
}
func correlatedCaptureFactory(
	callback func(CorrelatedAliasQuant) bool,
	flag bool,
) func(CorrelatedAliasQuant) bool {
	set := func() { flag = !flag }
	return func(value CorrelatedAliasQuant) bool {
		if flag {
			callback = correlatedAliasComplete
		}
		set()
		if flag {
			return callback(value)
		}
		return correlatedAliasComplete(value)
	}
}
func correlatedDormantFactory(
	callback func(CorrelatedAliasQuant) bool,
	flag bool,
) func(CorrelatedAliasQuant) bool {
	return func(value CorrelatedAliasQuant) bool {
		if flag {
			callback = correlatedAliasComplete
		}
		_ = func() { flag = !flag }
		if flag {
			return callback(value)
		}
		return correlatedAliasComplete(value)
	}
}
func CorrelatedAliasQMatMul(q CorrelatedAliasQuant) bool {
	callback := correlatedAliasFactory(correlatedAliasPartial, false)
	captured := correlatedCaptureFactory(correlatedCapturePartial, false)
	dormant := correlatedDormantFactory(correlatedDormantPartial, correlatedDormantFlag)
	return correlatedAliasComplete(q) && callback(q) && captured(q) && dormant(q)
}

var correlatedDormantFlag bool

// A called captured setter invalidates repeated-guard correlation even when
// the predicate's address is never taken.
type CorrelatedCaptureQuant uint8

const (
	CorrelatedCaptureA CorrelatedCaptureQuant = iota // want `quant variant CorrelatedCaptureA \(CorrelatedCaptureQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	CorrelatedCaptureB
	CorrelatedCaptureC
)

func correlatedCaptureByteSize(q CorrelatedCaptureQuant) int {
	switch q {
	case CorrelatedCaptureA, CorrelatedCaptureB, CorrelatedCaptureC:
		return 8
	default:
		return 0
	}
}
func correlatedCaptureDecode(q CorrelatedCaptureQuant) []float32 {
	switch q {
	case CorrelatedCaptureA, CorrelatedCaptureB, CorrelatedCaptureC:
		return []float32{}
	default:
		return nil
	}
}
func correlatedCaptureStandalonePartial(q CorrelatedCaptureQuant) bool {
	return q != CorrelatedCaptureA
}
func correlatedCaptureStandaloneComplete(q CorrelatedCaptureQuant) bool {
	return q == CorrelatedCaptureA || q == CorrelatedCaptureB || q == CorrelatedCaptureC
}
func correlatedCaptureStandaloneFactory(
	callback func(CorrelatedCaptureQuant) bool,
	flag bool,
) func(CorrelatedCaptureQuant) bool {
	set := func() { flag = !flag }
	invoke := set
	return func(value CorrelatedCaptureQuant) bool {
		if flag {
			callback = correlatedCaptureStandaloneComplete
		}
		invoke()
		if flag {
			return callback(value)
		}
		return correlatedCaptureStandaloneComplete(value)
	}
}
func CorrelatedCaptureQMatMul(q CorrelatedCaptureQuant) bool {
	callback := correlatedCaptureStandaloneFactory(correlatedCaptureStandalonePartial, false)
	return correlatedCaptureStandaloneComplete(q) && callback(q)
}

// A backend-named decoder is not portable decode evidence by itself.
type BackendDecodeOnlyQuant uint8

const (
	BackendDecodeOnlyA BackendDecodeOnlyQuant = iota
	BackendDecodeOnlyB
	BackendDecodeOnlyC
)

func backendOnlyByteSize(q BackendDecodeOnlyQuant) int {
	switch q {
	case BackendDecodeOnlyA, BackendDecodeOnlyB, BackendDecodeOnlyC:
		return 8
	default:
		return 0
	}
}
func cudaBackendDecodeOnlyDequantize(q BackendDecodeOnlyQuant) []float32 {
	switch q {
	case BackendDecodeOnlyA, BackendDecodeOnlyB, BackendDecodeOnlyC:
		return []float32{}
	default:
		return nil
	}
}
func cudaBackendOnlyQMatMul(q BackendDecodeOnlyQuant) bool {
	return cudaBackendDecodeOnlyDequantize(q) != nil
}
func backendOnlyPartialOne(q BackendDecodeOnlyQuant) bool {
	return q == BackendDecodeOnlyA || q == BackendDecodeOnlyC
}
func backendOnlyPartialTwo(q BackendDecodeOnlyQuant) bool {
	return q == BackendDecodeOnlyA || q == BackendDecodeOnlyC
}
func BackendOnlyQMatMul(q BackendDecodeOnlyQuant) bool {
	return backendOnlyPartialOne(q) && backendOnlyPartialTwo(q)
}

// Backend helpers reached from a portable decoder remain portable evidence.
type BackendDecodeBridgeQuant uint8

const (
	BackendDecodeBridgeA BackendDecodeBridgeQuant = iota
	BackendDecodeBridgeB                          // want `quant variant BackendDecodeBridgeB \(BackendDecodeBridgeQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	BackendDecodeBridgeC
)

func bridgeByteSize(q BackendDecodeBridgeQuant) int {
	switch q {
	case BackendDecodeBridgeA, BackendDecodeBridgeB, BackendDecodeBridgeC:
		return 8
	default:
		return 0
	}
}
func cudaBackendDecodeBridgeDequantize(q BackendDecodeBridgeQuant) []float32 {
	switch q {
	case BackendDecodeBridgeA, BackendDecodeBridgeB, BackendDecodeBridgeC:
		return []float32{}
	default:
		return nil
	}
}
func portableBridgeDecode(q BackendDecodeBridgeQuant) []float32 {
	return cudaBackendDecodeBridgeDequantize(q)
}
func bridgePartialOne(q BackendDecodeBridgeQuant) bool {
	return q == BackendDecodeBridgeA || q == BackendDecodeBridgeC
}
func bridgePartialTwo(q BackendDecodeBridgeQuant) bool {
	return q == BackendDecodeBridgeA || q == BackendDecodeBridgeC
}
func BridgeQMatMul(q BackendDecodeBridgeQuant) bool {
	return bridgePartialOne(q) && bridgePartialTwo(q)
}

// Backend context on a decoder method's receiver is not portable evidence.
type BackendReceiverQuant uint8

const (
	BackendReceiverA BackendReceiverQuant = iota
	BackendReceiverB
	BackendReceiverC
)

type cudaDecoder struct{}

func receiverBackendByteSize(q BackendReceiverQuant) int {
	switch q {
	case BackendReceiverA, BackendReceiverB, BackendReceiverC:
		return 8
	default:
		return 0
	}
}
func (cudaDecoder) Decode(q BackendReceiverQuant) []float32 {
	switch q {
	case BackendReceiverA, BackendReceiverB, BackendReceiverC:
		return []float32{}
	default:
		return nil
	}
}
func receiverBackendPartialOne(q BackendReceiverQuant) bool {
	return q == BackendReceiverA || q == BackendReceiverC
}
func receiverBackendPartialTwo(q BackendReceiverQuant) bool {
	return q == BackendReceiverA || q == BackendReceiverC
}
func ReceiverBackendQMatMul(q BackendReceiverQuant) bool {
	return receiverBackendPartialOne(q) && receiverBackendPartialTwo(q)
}

// Direct continue and goto exits do not execute the lexical switch
// continuation, even when that continuation reports success.
type DirectBranchExitQuant uint8

const (
	DirectBranchExitA DirectBranchExitQuant = iota
	DirectBranchExitB                       // want `quant variant DirectBranchExitB \(DirectBranchExitQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	DirectBranchExitC
)

func directBranchExitByteSize(q DirectBranchExitQuant) int {
	switch q {
	case DirectBranchExitA, DirectBranchExitB, DirectBranchExitC:
		return 8
	default:
		return 0
	}
}
func directBranchExitDecode(q DirectBranchExitQuant) []float32 {
	switch q {
	case DirectBranchExitA, DirectBranchExitB, DirectBranchExitC:
		return []float32{}
	default:
		return nil
	}
}
func directBranchExitContinue(q DirectBranchExitQuant) bool {
	for once := true; once; once = false {
		switch q {
		case DirectBranchExitA, DirectBranchExitC:
			return true
		case DirectBranchExitB:
			continue
		default:
			return false
		}
		return true
	}
	return false
}
func directBranchExitGoto(q DirectBranchExitQuant) bool {
	switch q {
	case DirectBranchExitA, DirectBranchExitC:
		return true
	case DirectBranchExitB:
		if false {
			goto supported
		}
		goto unsupported
	default:
		return false
	}
	return true
unsupported:
	return false
supported:
	return true
}
func DirectBranchExitQMatMul(q DirectBranchExitQuant) bool {
	return directBranchExitContinue(q) && directBranchExitGoto(q)
}

// Mixed non-fallthrough arms cannot inherit the switch continuation when one
// path continues or jumps away and every other path rejects the variant.
type MixedBranchExitQuant uint8

const (
	MixedBranchExitA MixedBranchExitQuant = iota
	MixedBranchExitB                      // want `quant variant MixedBranchExitB \(MixedBranchExitQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	MixedBranchExitC
)

func mixedBranchExitByteSize(q MixedBranchExitQuant) int {
	switch q {
	case MixedBranchExitA, MixedBranchExitB, MixedBranchExitC:
		return 8
	default:
		return 0
	}
}
func mixedBranchExitDecode(q MixedBranchExitQuant) []float32 {
	switch q {
	case MixedBranchExitA, MixedBranchExitB, MixedBranchExitC:
		return []float32{}
	default:
		return nil
	}
}
func mixedBranchExitContinue(q MixedBranchExitQuant, skip bool) bool {
	for once := true; once; once = false {
		switch q {
		case MixedBranchExitA, MixedBranchExitC:
			return true
		case MixedBranchExitB:
			if skip {
				continue
			} else {
				return false
			}
		default:
			return false
		}
		return true
	}
	return false
}
func mixedBranchExitGoto(q MixedBranchExitQuant, skip bool) bool {
	switch q {
	case MixedBranchExitA, MixedBranchExitC:
		return true
	case MixedBranchExitB:
		if skip {
			goto unsupported
		}
		return false
	default:
		return false
	}
	return true
unsupported:
	return false
}
func MixedBranchExitQMatMul(q MixedBranchExitQuant) bool {
	return mixedBranchExitContinue(q, true) && mixedBranchExitGoto(q, true)
}

// A goto can bypass the lexical switch continuation while still reaching an
// independently successful label.
type GotoSupportedQuant uint8

const (
	GotoSupportedA GotoSupportedQuant = iota
	GotoSupportedB
	GotoSupportedC
)

func gotoSupportedByteSize(q GotoSupportedQuant) int {
	switch q {
	case GotoSupportedA, GotoSupportedB, GotoSupportedC:
		return 8
	default:
		return 0
	}
}
func gotoSupportedDecode(q GotoSupportedQuant) []float32 {
	switch q {
	case GotoSupportedA, GotoSupportedB, GotoSupportedC:
		return []float32{}
	default:
		return nil
	}
}
func gotoSupportedOne(q GotoSupportedQuant) bool {
	switch q {
	case GotoSupportedA, GotoSupportedC:
		return true
	case GotoSupportedB:
		goto supported
	default:
		return false
	}
	return true
supported:
	return true
}
func gotoSupportedTwo(q GotoSupportedQuant) bool {
	switch q {
	case GotoSupportedA, GotoSupportedC:
		return true
	case GotoSupportedB:
		goto supported
	default:
		return false
	}
	return true
supported:
	return true
}
func GotoSupportedQMatMul(q GotoSupportedQuant) bool {
	return gotoSupportedOne(q) && gotoSupportedTwo(q)
}

// Constant and case-correlated conditions can make a terminal continue the
// only reachable branch, so the post-switch success remains unreachable.
type CaseContinueQuant uint8

const (
	CaseContinueA CaseContinueQuant = iota
	CaseContinueB                   // want `quant variant CaseContinueB \(CaseContinueQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	CaseContinueC
)

func caseContinueByteSize(q CaseContinueQuant) int {
	switch q {
	case CaseContinueA, CaseContinueB, CaseContinueC:
		return 8
	default:
		return 0
	}
}
func caseContinueDecode(q CaseContinueQuant) []float32 {
	switch q {
	case CaseContinueA, CaseContinueB, CaseContinueC:
		return []float32{}
	default:
		return nil
	}
}
func caseContinueConstant(q CaseContinueQuant) bool {
	for once := true; once; once = false {
		switch q {
		case CaseContinueA, CaseContinueC:
			return true
		case CaseContinueB:
			if true {
				continue
			}
		default:
			return false
		}
		return true
	}
	return false
}
func caseContinueCorrelated(q CaseContinueQuant) bool {
	for once := true; once; once = false {
		switch q {
		case CaseContinueA, CaseContinueC:
			return true
		case CaseContinueB:
			if q == CaseContinueB {
				continue
			}
		default:
			return false
		}
		return true
	}
	return false
}
func CaseContinueQMatMul(q CaseContinueQuant) bool {
	return caseContinueConstant(q) && caseContinueCorrelated(q)
}

// Multi-value switch clauses are evaluated separately for each enum value.
type MultiCaseContinueQuant uint8

const (
	MultiCaseContinueA MultiCaseContinueQuant = iota
	MultiCaseContinueB                        // want `quant variant MultiCaseContinueB \(MultiCaseContinueQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	MultiCaseContinueC
)

func multiCaseContinueByteSize(q MultiCaseContinueQuant) int {
	switch q {
	case MultiCaseContinueA, MultiCaseContinueB, MultiCaseContinueC:
		return 8
	default:
		return 0
	}
}
func multiCaseContinueDecode(q MultiCaseContinueQuant) []float32 {
	switch q {
	case MultiCaseContinueA, MultiCaseContinueB, MultiCaseContinueC:
		return []float32{}
	default:
		return nil
	}
}
func multiCaseContinueOne(q MultiCaseContinueQuant) bool {
	for once := true; once; once = false {
		switch q {
		case MultiCaseContinueA, MultiCaseContinueB:
			if q == MultiCaseContinueB {
				continue
			}
			return true
		case MultiCaseContinueC:
			return true
		default:
			return false
		}
		return true
	}
	return false
}
func multiCaseContinueTwo(q MultiCaseContinueQuant) bool {
	for once := true; once; once = false {
		switch q {
		case MultiCaseContinueA, MultiCaseContinueB:
			if q == MultiCaseContinueB {
				continue
			}
			return true
		case MultiCaseContinueC:
			return true
		default:
			return false
		}
		return true
	}
	return false
}
func MultiCaseContinueQMatMul(q MultiCaseContinueQuant) bool {
	return multiCaseContinueOne(q) && multiCaseContinueTwo(q)
}

// Default and tagless cases retain exact enum constraints when deciding
// whether a terminal continue can reach the switch continuation.
type DefaultTaglessContinueQuant uint8

const (
	DefaultTaglessContinueA DefaultTaglessContinueQuant = iota
	DefaultTaglessContinueB                             // want `quant variant DefaultTaglessContinueB \(DefaultTaglessContinueQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	DefaultTaglessContinueC
)

func defaultTaglessContinueByteSize(q DefaultTaglessContinueQuant) int {
	switch q {
	case DefaultTaglessContinueA, DefaultTaglessContinueB, DefaultTaglessContinueC:
		return 8
	default:
		return 0
	}
}
func defaultTaglessContinueDecode(q DefaultTaglessContinueQuant) []float32 {
	switch q {
	case DefaultTaglessContinueA, DefaultTaglessContinueB, DefaultTaglessContinueC:
		return []float32{}
	default:
		return nil
	}
}
func defaultTaglessContinueTagged(q DefaultTaglessContinueQuant) bool {
	for once := true; once; once = false {
		switch q {
		case DefaultTaglessContinueA, DefaultTaglessContinueC:
			return true
		default:
			if q == DefaultTaglessContinueB {
				continue
			}
			return false
		}
		return true
	}
	return false
}
func defaultTaglessContinueGuard(q DefaultTaglessContinueQuant) bool {
	for once := true; once; once = false {
		switch {
		case q == DefaultTaglessContinueA, q == DefaultTaglessContinueC:
			return true
		case q == DefaultTaglessContinueB:
			if q == DefaultTaglessContinueB {
				continue
			}
		default:
			return false
		}
		return true
	}
	return false
}
func DefaultTaglessContinueQMatMul(q DefaultTaglessContinueQuant) bool {
	return defaultTaglessContinueTagged(q) && defaultTaglessContinueGuard(q)
}

// Compound and open tagless guards are evaluated over their finite enum
// domains instead of being treated as one unconstrained case.
type ComplexTaglessContinueQuant uint8

const (
	ComplexTaglessContinueA ComplexTaglessContinueQuant = iota
	ComplexTaglessContinueB                             // want `quant variant ComplexTaglessContinueB \(ComplexTaglessContinueQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	ComplexTaglessContinueC
)

func complexTaglessContinueByteSize(q ComplexTaglessContinueQuant) int {
	switch q {
	case ComplexTaglessContinueA, ComplexTaglessContinueB, ComplexTaglessContinueC:
		return 8
	default:
		return 0
	}
}
func complexTaglessContinueDecode(q ComplexTaglessContinueQuant) []float32 {
	switch q {
	case ComplexTaglessContinueA, ComplexTaglessContinueB, ComplexTaglessContinueC:
		return []float32{}
	default:
		return nil
	}
}
func complexTaglessContinueConjunction(q ComplexTaglessContinueQuant, enabled bool) bool {
	for once := true; once; once = false {
		switch {
		case q == ComplexTaglessContinueA, q == ComplexTaglessContinueC:
			return true
		case enabled && q == ComplexTaglessContinueB:
			continue
		default:
			return false
		}
		return true
	}
	return false
}
func complexTaglessContinueOpen(q ComplexTaglessContinueQuant) bool {
	for once := true; once; once = false {
		switch {
		case q == ComplexTaglessContinueA:
			return true
		case q != ComplexTaglessContinueA:
			if q == ComplexTaglessContinueB {
				continue
			}
			return true
		default:
			return false
		}
		return true
	}
	return false
}
func ComplexTaglessContinueQMatMul(q ComplexTaglessContinueQuant) bool {
	return complexTaglessContinueConjunction(q, true) && complexTaglessContinueOpen(q)
}

// Stable direct and converted aliases share the source case's enum value.
type AliasCaseContinueQuant uint8

const (
	AliasCaseContinueA AliasCaseContinueQuant = iota
	AliasCaseContinueB                        // want `quant variant AliasCaseContinueB \(AliasCaseContinueQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	AliasCaseContinueC
)

func aliasCaseContinueByteSize(q AliasCaseContinueQuant) int {
	switch q {
	case AliasCaseContinueA, AliasCaseContinueB, AliasCaseContinueC:
		return 8
	default:
		return 0
	}
}
func aliasCaseContinueDecode(q AliasCaseContinueQuant) []float32 {
	switch q {
	case AliasCaseContinueA, AliasCaseContinueB, AliasCaseContinueC:
		return []float32{}
	default:
		return nil
	}
}
func aliasCaseContinueDirect(q AliasCaseContinueQuant) bool {
	alias := q
	for once := true; once; once = false {
		switch q {
		case AliasCaseContinueA, AliasCaseContinueC:
			return true
		case AliasCaseContinueB:
			if alias == AliasCaseContinueB {
				continue
			}
		default:
			return false
		}
		return true
	}
	return false
}
func aliasCaseContinueConverted(q AliasCaseContinueQuant) bool {
	alias := AliasCaseContinueQuant(q)
	for once := true; once; once = false {
		switch q {
		case AliasCaseContinueA, AliasCaseContinueC:
			return true
		case AliasCaseContinueB:
			if alias == AliasCaseContinueB {
				continue
			}
		default:
			return false
		}
		return true
	}
	return false
}
func AliasCaseContinueQMatMul(q AliasCaseContinueQuant) bool {
	return aliasCaseContinueDirect(q) && aliasCaseContinueConverted(q)
}

// Goto target outcomes are evaluated under the source case's enum value.
type ScopedGotoQuant uint8

const (
	ScopedGotoA ScopedGotoQuant = iota
	ScopedGotoB                 // want `quant variant ScopedGotoB \(ScopedGotoQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	ScopedGotoC
)

func scopedGotoByteSize(q ScopedGotoQuant) int {
	switch q {
	case ScopedGotoA, ScopedGotoB, ScopedGotoC:
		return 8
	default:
		return 0
	}
}
func scopedGotoDecode(q ScopedGotoQuant) []float32 {
	switch q {
	case ScopedGotoA, ScopedGotoB, ScopedGotoC:
		return []float32{}
	default:
		return nil
	}
}
func scopedGotoOne(q ScopedGotoQuant) bool {
	switch q {
	case ScopedGotoA, ScopedGotoC:
		return true
	case ScopedGotoB:
		goto target
	default:
		return false
	}
	return false
target:
	if q == ScopedGotoA {
		return true
	}
	return false
}
func scopedGotoTwo(q ScopedGotoQuant) bool {
	switch q {
	case ScopedGotoA, ScopedGotoC:
		return true
	case ScopedGotoB:
		goto target
	default:
		return false
	}
	return false
target:
	if q == ScopedGotoC {
		return true
	}
	return false
}
func ScopedGotoQMatMul(q ScopedGotoQuant) bool {
	return scopedGotoOne(q) && scopedGotoTwo(q)
}

// Named goto-target results are resolved under the source case value.
type NamedScopedGotoQuant uint8

const (
	NamedScopedGotoA NamedScopedGotoQuant = iota
	NamedScopedGotoB                      // want `quant variant NamedScopedGotoB \(NamedScopedGotoQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	NamedScopedGotoC
)

func namedScopedGotoByteSize(q NamedScopedGotoQuant) int {
	switch q {
	case NamedScopedGotoA, NamedScopedGotoB, NamedScopedGotoC:
		return 8
	default:
		return 0
	}
}
func namedScopedGotoDecode(q NamedScopedGotoQuant) []float32 {
	switch q {
	case NamedScopedGotoA, NamedScopedGotoB, NamedScopedGotoC:
		return []float32{}
	default:
		return nil
	}
}
func namedScopedGotoOne(q NamedScopedGotoQuant) (ok bool) {
	switch q {
	case NamedScopedGotoA, NamedScopedGotoC:
		return true
	case NamedScopedGotoB:
		goto target
	default:
		return false
	}
target:
	ok = q == NamedScopedGotoA
	return
}
func namedScopedGotoTwo(q NamedScopedGotoQuant) (ok bool) {
	switch q {
	case NamedScopedGotoA, NamedScopedGotoC:
		return true
	case NamedScopedGotoB:
		goto target
	default:
		return false
	}
target:
	ok = q == NamedScopedGotoC
	return
}
func NamedScopedGotoQMatMul(q NamedScopedGotoQuant) bool {
	return namedScopedGotoOne(q) && namedScopedGotoTwo(q)
}

// An independent successful result keeps a scoped goto target handled.
type MultiResultScopedGotoQuant uint8

const (
	MultiResultScopedGotoA MultiResultScopedGotoQuant = iota
	MultiResultScopedGotoB
	MultiResultScopedGotoC
)

func multiResultScopedGotoByteSize(q MultiResultScopedGotoQuant) int {
	switch q {
	case MultiResultScopedGotoA, MultiResultScopedGotoB, MultiResultScopedGotoC:
		return 8
	default:
		return 0
	}
}
func multiResultScopedGotoDecode(q MultiResultScopedGotoQuant) []float32 {
	switch q {
	case MultiResultScopedGotoA, MultiResultScopedGotoB, MultiResultScopedGotoC:
		return []float32{}
	default:
		return nil
	}
}
func multiResultScopedGotoOne(q MultiResultScopedGotoQuant) (bool, int) {
	switch q {
	case MultiResultScopedGotoA, MultiResultScopedGotoC:
		return true, 1
	case MultiResultScopedGotoB:
		goto target
	default:
		return false, 0
	}
target:
	return q == MultiResultScopedGotoA, 1
}
func multiResultScopedGotoTwo(q MultiResultScopedGotoQuant) (bool, int) {
	switch q {
	case MultiResultScopedGotoA, MultiResultScopedGotoC:
		return true, 1
	case MultiResultScopedGotoB:
		goto target
	default:
		return false, 0
	}
target:
	return q == MultiResultScopedGotoC, 1
}
func MultiResultScopedGotoQMatMul(q MultiResultScopedGotoQuant) bool {
	one, _ := multiResultScopedGotoOne(q)
	two, _ := multiResultScopedGotoTwo(q)
	return one && two
}

// Invoked named and local setters invalidate a source-case assumption before
// a goto target tests the mutated subject.
type CallMutationGotoQuant uint8

const (
	CallMutationGotoA CallMutationGotoQuant = iota
	CallMutationGotoB
	CallMutationGotoC
)

var callMutationGotoCurrent CallMutationGotoQuant

func callMutationGotoByteSize(q CallMutationGotoQuant) int {
	switch q {
	case CallMutationGotoA, CallMutationGotoB, CallMutationGotoC:
		return 8
	default:
		return 0
	}
}
func callMutationGotoDecode(q CallMutationGotoQuant) []float32 {
	switch q {
	case CallMutationGotoA, CallMutationGotoB, CallMutationGotoC:
		return []float32{}
	default:
		return nil
	}
}
func callMutationGotoSetA() { callMutationGotoCurrent = CallMutationGotoA }
func callMutationGotoNamed(q CallMutationGotoQuant) bool {
	callMutationGotoCurrent = q
	switch callMutationGotoCurrent {
	case CallMutationGotoA, CallMutationGotoC:
		return true
	case CallMutationGotoB:
		goto target
	default:
		return false
	}
target:
	callMutationGotoSetA()
	return callMutationGotoCurrent == CallMutationGotoA
}
func callMutationGotoLocal(q CallMutationGotoQuant) bool {
	setA := func() { q = CallMutationGotoA }
	switch q {
	case CallMutationGotoA, CallMutationGotoC:
		return true
	case CallMutationGotoB:
		goto target
	default:
		return false
	}
target:
	setA()
	return q == CallMutationGotoA
}
func CallMutationGotoQMatMul(q CallMutationGotoQuant) bool {
	return callMutationGotoNamed(q) && callMutationGotoLocal(q)
}

// A void goto target at the function end succeeds through its implicit
// return, even though it contains no explicit return statement.
type VoidGotoQuant uint8

const (
	VoidGotoA VoidGotoQuant = iota
	VoidGotoB
	VoidGotoC
)

func voidGotoByteSize(q VoidGotoQuant) int {
	switch q {
	case VoidGotoA, VoidGotoB, VoidGotoC:
		return 8
	default:
		return 0
	}
}
func voidGotoDecode(q VoidGotoQuant) []float32 {
	switch q {
	case VoidGotoA, VoidGotoB, VoidGotoC:
		return []float32{}
	default:
		return nil
	}
}
func voidGotoOne(q VoidGotoQuant) {
	switch q {
	case VoidGotoA, VoidGotoC:
		return
	case VoidGotoB:
		goto supported
	default:
		panic("unsupported")
	}
	panic("unreachable continuation")
supported:
}
func voidGotoTwo(q VoidGotoQuant) {
	switch q {
	case VoidGotoA, VoidGotoC:
		return
	case VoidGotoB:
		goto supported
	default:
		panic("unsupported")
	}
	panic("unreachable continuation")
supported:
}
func VoidGotoQMatMul(q VoidGotoQuant) {
	voidGotoOne(q)
	voidGotoTwo(q)
}

// Transitive stable closures invalidate the source-case assumption before a
// goto target observes the captured mutation.
type TransitiveMutationQuant uint8

const (
	TransitiveMutationA TransitiveMutationQuant = iota
	TransitiveMutationB
	TransitiveMutationC
)

func transitiveMutationByteSize(q TransitiveMutationQuant) int {
	if q == TransitiveMutationA || q == TransitiveMutationB || q == TransitiveMutationC {
		return 8
	}
	return 0
}
func transitiveMutationDecode(q TransitiveMutationQuant) []float32 {
	if q == TransitiveMutationA || q == TransitiveMutationB || q == TransitiveMutationC {
		return []float32{}
	}
	return nil
}
func transitiveMutationOne(q TransitiveMutationQuant) bool {
	inner := func() { q = TransitiveMutationA }
	outer := func() { inner() }
	switch q {
	case TransitiveMutationA, TransitiveMutationC:
		return true
	case TransitiveMutationB:
		goto done
	default:
		return false
	}
done:
	outer()
	return q == TransitiveMutationA
}
func transitiveMutationTwo(q TransitiveMutationQuant) bool {
	inner := func() { q = TransitiveMutationA }
	outer := func() { inner() }
	switch q {
	case TransitiveMutationA, TransitiveMutationC:
		return true
	case TransitiveMutationB:
		goto done
	default:
		return false
	}
done:
	outer()
	return q == TransitiveMutationA
}
func TransitiveMutationQMatMul(q TransitiveMutationQuant) bool {
	return transitiveMutationOne(q) && transitiveMutationTwo(q)
}

// Proven-empty same-package calls preserve a package-global case assumption.
type BenignCallQuant uint8

const (
	BenignCallA BenignCallQuant = iota
	BenignCallB                 // want `quant variant BenignCallB \(BenignCallQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	BenignCallC
)

var benignCallValue BenignCallQuant

func benignCallByteSize(q BenignCallQuant) int {
	if q == BenignCallA || q == BenignCallB || q == BenignCallC {
		return 8
	}
	return 0
}
func benignCallDecode(q BenignCallQuant) []float32 {
	if q == BenignCallA || q == BenignCallB || q == BenignCallC {
		return []float32{}
	}
	return nil
}
func benignCallNoop() {}
func benignCallOne(q BenignCallQuant) bool {
	benignCallValue = q
	switch benignCallValue {
	case BenignCallA, BenignCallC:
		return true
	case BenignCallB:
		goto done
	default:
		return false
	}
done:
	benignCallNoop()
	return benignCallValue == BenignCallA
}
func benignCallTwo(q BenignCallQuant) bool {
	benignCallValue = q
	switch benignCallValue {
	case BenignCallA, BenignCallC:
		return true
	case BenignCallB:
		goto done
	default:
		return false
	}
done:
	benignCallNoop()
	return benignCallValue == BenignCallC
}
func BenignCallQMatMul(q BenignCallQuant) bool {
	return benignCallOne(q) && benignCallTwo(q)
}

// Side effects in earlier tagless predicates flow into later cases/defaults.
type DispatchMutationQuant uint8

const (
	DispatchMutationA DispatchMutationQuant = iota
	DispatchMutationB
	DispatchMutationC
)

func dispatchMutationByteSize(q DispatchMutationQuant) int {
	if q == DispatchMutationA || q == DispatchMutationB || q == DispatchMutationC {
		return 8
	}
	return 0
}
func dispatchMutationDecode(q DispatchMutationQuant) []float32 {
	if q == DispatchMutationA || q == DispatchMutationB || q == DispatchMutationC {
		return []float32{}
	}
	return nil
}
func dispatchMutationOne(q DispatchMutationQuant) bool {
	switch {
	case func() bool { q = DispatchMutationA; return false }() && q == DispatchMutationB:
		return false
	case q == DispatchMutationA, q == DispatchMutationC:
		return true
	default:
		return q == DispatchMutationA
	}
}
func dispatchMutationTwo(q DispatchMutationQuant) bool {
	switch {
	case func() bool { q = DispatchMutationC; return false }() && q == DispatchMutationB:
		return false
	case q == DispatchMutationA, q == DispatchMutationC:
		return true
	default:
		return q == DispatchMutationC
	}
}
func DispatchMutationQMatMul(q DispatchMutationQuant) bool {
	return dispatchMutationOne(q) && dispatchMutationTwo(q)
}

// Side effects in the matching predicate also flow into the selected body.
type MatchingMutationQuant uint8

const (
	MatchingMutationA MatchingMutationQuant = iota
	MatchingMutationB
	MatchingMutationC
)

func matchingMutationByteSize(q MatchingMutationQuant) int {
	if q == MatchingMutationA || q == MatchingMutationB || q == MatchingMutationC {
		return 8
	}
	return 0
}
func matchingMutationDecode(q MatchingMutationQuant) []float32 {
	if q == MatchingMutationA || q == MatchingMutationB || q == MatchingMutationC {
		return []float32{}
	}
	return nil
}
func matchingMutationOne(q MatchingMutationQuant) bool {
	switch {
	case q == MatchingMutationB && func() bool { q = MatchingMutationA; return true }():
		return q == MatchingMutationA
	case q == MatchingMutationA, q == MatchingMutationC:
		return true
	default:
		return false
	}
}
func matchingMutationTwo(q MatchingMutationQuant) bool {
	switch {
	case q == MatchingMutationB && func() bool { q = MatchingMutationC; return true }():
		return q == MatchingMutationC
	case q == MatchingMutationA, q == MatchingMutationC:
		return true
	default:
		return false
	}
}
func MatchingMutationQMatMul(q MatchingMutationQuant) bool {
	return matchingMutationOne(q) && matchingMutationTwo(q)
}

// Per-domain default resolution does not suppress the same default for an
// unrelated enum whose tagless subjects are ambiguous.
type CrossEnumDefaultPrimary uint8
type CrossEnumDefaultQuant uint8

const (
	CrossEnumDefaultPrimaryA CrossEnumDefaultPrimary = iota
	CrossEnumDefaultPrimaryB
	CrossEnumDefaultPrimaryC
)
const (
	CrossEnumDefaultA CrossEnumDefaultQuant = iota
	CrossEnumDefaultB
	CrossEnumDefaultC
)

func crossEnumDefaultByteSize(q CrossEnumDefaultQuant) int {
	if q == CrossEnumDefaultA || q == CrossEnumDefaultB || q == CrossEnumDefaultC {
		return 8
	}
	return 0
}
func crossEnumDefaultDecode(q CrossEnumDefaultQuant) []float32 {
	if q == CrossEnumDefaultA || q == CrossEnumDefaultB || q == CrossEnumDefaultC {
		return []float32{}
	}
	return nil
}
func crossEnumDefaultOne(p CrossEnumDefaultPrimary, q, alias CrossEnumDefaultQuant) bool {
	switch {
	case p == CrossEnumDefaultPrimaryA:
		return true
	case q == CrossEnumDefaultA:
		return true
	case alias == CrossEnumDefaultC:
		return true
	default:
		return true
	}
}
func crossEnumDefaultTwo(p CrossEnumDefaultPrimary, q, alias CrossEnumDefaultQuant) bool {
	switch {
	case p == CrossEnumDefaultPrimaryA:
		return true
	case q == CrossEnumDefaultA:
		return true
	case alias == CrossEnumDefaultC:
		return true
	default:
		return true
	}
}
func CrossEnumDefaultQMatMul(q CrossEnumDefaultQuant, p CrossEnumDefaultPrimary) bool {
	return crossEnumDefaultOne(p, q, q) && crossEnumDefaultTwo(p, q, q)
}

// Stable pointer actuals are projected to callee formals before mutation
// effects are summarized.
type PointerAliasMutationQuant uint8

const (
	PointerAliasMutationA PointerAliasMutationQuant = iota
	PointerAliasMutationB
	PointerAliasMutationC
)

func pointerAliasMutationByteSize(q PointerAliasMutationQuant) int {
	if q == PointerAliasMutationA || q == PointerAliasMutationB || q == PointerAliasMutationC {
		return 8
	}
	return 0
}
func pointerAliasMutationDecode(q PointerAliasMutationQuant) []float32 {
	if q == PointerAliasMutationA || q == PointerAliasMutationB || q == PointerAliasMutationC {
		return []float32{}
	}
	return nil
}
func pointerAliasMutationSet(pointer *PointerAliasMutationQuant, value PointerAliasMutationQuant) {
	*pointer = value
}
func pointerAliasMutationOne(q PointerAliasMutationQuant) bool {
	pointer := &q
	switch q {
	case PointerAliasMutationA, PointerAliasMutationC:
		return true
	case PointerAliasMutationB:
		goto done
	default:
		return false
	}
done:
	pointerAliasMutationSet(pointer, PointerAliasMutationA)
	return q == PointerAliasMutationA
}
func pointerAliasMutationTwo(q PointerAliasMutationQuant) bool {
	pointer := &q
	switch q {
	case PointerAliasMutationA, PointerAliasMutationC:
		return true
	case PointerAliasMutationB:
		goto done
	default:
		return false
	}
done:
	pointerAliasMutationSet(pointer, PointerAliasMutationC)
	return q == PointerAliasMutationC
}
func PointerAliasMutationQMatMul(q PointerAliasMutationQuant) bool {
	return pointerAliasMutationOne(q) && pointerAliasMutationTwo(q)
}

// Pointer-bearing selector actuals conservatively invalidate the caller
// assumption when their pointee cannot be proven disjoint.
type HolderAliasMutationQuant uint8

const (
	HolderAliasMutationA HolderAliasMutationQuant = iota
	HolderAliasMutationB
	HolderAliasMutationC
)

type holderAliasMutationBox struct{ pointer *HolderAliasMutationQuant }

func holderAliasMutationByteSize(q HolderAliasMutationQuant) int {
	if q == HolderAliasMutationA || q == HolderAliasMutationB || q == HolderAliasMutationC {
		return 8
	}
	return 0
}
func holderAliasMutationDecode(q HolderAliasMutationQuant) []float32 {
	if q == HolderAliasMutationA || q == HolderAliasMutationB || q == HolderAliasMutationC {
		return []float32{}
	}
	return nil
}
func holderAliasMutationSet(pointer *HolderAliasMutationQuant, value HolderAliasMutationQuant) {
	*pointer = value
}
func holderAliasMutationOne(q HolderAliasMutationQuant) bool {
	holder := holderAliasMutationBox{pointer: &q}
	switch q {
	case HolderAliasMutationA, HolderAliasMutationC:
		return true
	case HolderAliasMutationB:
		goto done
	default:
		return false
	}
done:
	holderAliasMutationSet(holder.pointer, HolderAliasMutationA)
	return q == HolderAliasMutationA
}
func holderAliasMutationTwo(q HolderAliasMutationQuant) bool {
	holder := holderAliasMutationBox{pointer: &q}
	switch q {
	case HolderAliasMutationA, HolderAliasMutationC:
		return true
	case HolderAliasMutationB:
		goto done
	default:
		return false
	}
done:
	holderAliasMutationSet(holder.pointer, HolderAliasMutationC)
	return q == HolderAliasMutationC
}
func HolderAliasMutationQMatMul(q HolderAliasMutationQuant) bool {
	return holderAliasMutationOne(q) && holderAliasMutationTwo(q)
}

// Dispatch-effect invalidation preserves the stable parameter identity used
// to apply a fixed interprocedural call scope.
type MutatedSpecializedQuant uint8

const (
	MutatedSpecializedA MutatedSpecializedQuant = iota
	MutatedSpecializedB
	MutatedSpecializedC
)

func mutatedSpecializedByteSize(q MutatedSpecializedQuant) int {
	if q == MutatedSpecializedA || q == MutatedSpecializedB || q == MutatedSpecializedC {
		return 8
	}
	return 0
}
func mutatedSpecializedDecode(q MutatedSpecializedQuant) []float32 {
	if q == MutatedSpecializedA || q == MutatedSpecializedB || q == MutatedSpecializedC {
		return []float32{}
	}
	return nil
}
func mutatedSpecializedOne(q MutatedSpecializedQuant) bool {
	switch {
	case q == MutatedSpecializedB && func() bool { q = MutatedSpecializedA; return true }():
		return true
	case q == MutatedSpecializedA:
		return true
	default:
		return false
	}
}
func mutatedSpecializedTwo(q MutatedSpecializedQuant) bool {
	switch {
	case q == MutatedSpecializedB && func() bool { q = MutatedSpecializedA; return true }():
		return true
	case q == MutatedSpecializedA:
		return true
	default:
		return false
	}
}
func MutatedSpecializedQMatMul(q MutatedSpecializedQuant) bool {
	return mutatedSpecializedOne(MutatedSpecializedA) && mutatedSpecializedTwo(MutatedSpecializedA)
}

// unsafe.Pointer and uintptr round trips remain reference-capable when a
// stable local is passed to another same-package helper.
type UnsafeAliasMutationQuant uint8

const (
	UnsafeAliasMutationA UnsafeAliasMutationQuant = iota
	UnsafeAliasMutationB
	UnsafeAliasMutationC
)

func unsafeAliasMutationByteSize(q UnsafeAliasMutationQuant) int {
	if q == UnsafeAliasMutationA || q == UnsafeAliasMutationB || q == UnsafeAliasMutationC {
		return 8
	}
	return 0
}
func unsafeAliasMutationDecode(q UnsafeAliasMutationQuant) []float32 {
	if q == UnsafeAliasMutationA || q == UnsafeAliasMutationB || q == UnsafeAliasMutationC {
		return []float32{}
	}
	return nil
}
func unsafeAliasMutationPointer(q *UnsafeAliasMutationQuant) unsafe.Pointer {
	return unsafe.Pointer(q)
}
func unsafeAliasMutationSet(pointer unsafe.Pointer, value UnsafeAliasMutationQuant) {
	*(*UnsafeAliasMutationQuant)(pointer) = value
}
func unsafeAliasMutationOne(q UnsafeAliasMutationQuant) bool {
	pointer := unsafeAliasMutationPointer(&q)
	switch q {
	case UnsafeAliasMutationA, UnsafeAliasMutationC:
		return true
	case UnsafeAliasMutationB:
		goto done
	default:
		return false
	}
done:
	unsafeAliasMutationSet(pointer, UnsafeAliasMutationA)
	return q == UnsafeAliasMutationA
}
func unsafeAliasMutationTwo(q UnsafeAliasMutationQuant) bool {
	pointer := unsafeAliasMutationPointer(&q)
	switch q {
	case UnsafeAliasMutationA, UnsafeAliasMutationC:
		return true
	case UnsafeAliasMutationB:
		goto done
	default:
		return false
	}
done:
	unsafeAliasMutationSet(pointer, UnsafeAliasMutationC)
	return q == UnsafeAliasMutationC
}
func UnsafeAliasMutationQMatMul(q UnsafeAliasMutationQuant) bool {
	return unsafeAliasMutationOne(q) && unsafeAliasMutationTwo(q)
}

type UintptrAliasMutationQuant uint8

const (
	UintptrAliasMutationA UintptrAliasMutationQuant = iota
	UintptrAliasMutationB
	UintptrAliasMutationC
)

func uintptrAliasMutationByteSize(q UintptrAliasMutationQuant) int {
	if q == UintptrAliasMutationA || q == UintptrAliasMutationB || q == UintptrAliasMutationC {
		return 8
	}
	return 0
}
func uintptrAliasMutationDecode(q UintptrAliasMutationQuant) []float32 {
	if q == UintptrAliasMutationA || q == UintptrAliasMutationB || q == UintptrAliasMutationC {
		return []float32{}
	}
	return nil
}
func uintptrAliasMutationPointer(q *UintptrAliasMutationQuant) uintptr {
	return uintptr(unsafe.Pointer(q))
}
func uintptrAliasMutationSet(pointer uintptr, value UintptrAliasMutationQuant) {
	*(*UintptrAliasMutationQuant)(unsafe.Pointer(pointer)) = value
}
func uintptrAliasMutationOne(q UintptrAliasMutationQuant) bool {
	pointer := uintptrAliasMutationPointer(&q)
	switch q {
	case UintptrAliasMutationA, UintptrAliasMutationC:
		return true
	case UintptrAliasMutationB:
		goto done
	default:
		return false
	}
done:
	uintptrAliasMutationSet(pointer, UintptrAliasMutationA)
	return q == UintptrAliasMutationA
}
func uintptrAliasMutationTwo(q UintptrAliasMutationQuant) bool {
	pointer := uintptrAliasMutationPointer(&q)
	switch q {
	case UintptrAliasMutationA, UintptrAliasMutationC:
		return true
	case UintptrAliasMutationB:
		goto done
	default:
		return false
	}
done:
	uintptrAliasMutationSet(pointer, UintptrAliasMutationC)
	return q == UintptrAliasMutationC
}
func UintptrAliasMutationQMatMul(q UintptrAliasMutationQuant) bool {
	return uintptrAliasMutationOne(q) && uintptrAliasMutationTwo(q)
}

// A tuple-valued argument is projected element-by-element onto the callee's
// formals, including a pointer formal that is not the first result.
type TupleAliasMutationQuant uint8

const (
	TupleAliasMutationA TupleAliasMutationQuant = iota
	TupleAliasMutationB
	TupleAliasMutationC
)

func tupleAliasMutationByteSize(q TupleAliasMutationQuant) int {
	if q == TupleAliasMutationA || q == TupleAliasMutationB || q == TupleAliasMutationC {
		return 8
	}
	return 0
}
func tupleAliasMutationDecode(q TupleAliasMutationQuant) []float32 {
	if q == TupleAliasMutationA || q == TupleAliasMutationB || q == TupleAliasMutationC {
		return []float32{}
	}
	return nil
}
func tupleAliasMutationPointer(
	q *TupleAliasMutationQuant,
	value TupleAliasMutationQuant,
) (int, *TupleAliasMutationQuant, TupleAliasMutationQuant) {
	return 0, q, value
}
func tupleAliasMutationSet(_ int, pointer *TupleAliasMutationQuant, value TupleAliasMutationQuant) {
	*pointer = value
}
func tupleAliasMutationOne(q TupleAliasMutationQuant) bool {
	pointer := &q
	switch q {
	case TupleAliasMutationA, TupleAliasMutationC:
		return true
	case TupleAliasMutationB:
		goto done
	default:
		return false
	}
done:
	tupleAliasMutationSet(tupleAliasMutationPointer(pointer, TupleAliasMutationA))
	return q == TupleAliasMutationA
}
func tupleAliasMutationTwo(q TupleAliasMutationQuant) bool {
	pointer := &q
	switch q {
	case TupleAliasMutationA, TupleAliasMutationC:
		return true
	case TupleAliasMutationB:
		goto done
	default:
		return false
	}
done:
	tupleAliasMutationSet(tupleAliasMutationPointer(pointer, TupleAliasMutationC))
	return q == TupleAliasMutationC
}
func TupleAliasMutationQMatMul(q TupleAliasMutationQuant) bool {
	return tupleAliasMutationOne(q) && tupleAliasMutationTwo(q)
}

var _ = []any{
	transitiveMutationByteSize, transitiveMutationDecode, TransitiveMutationQMatMul,
	benignCallByteSize, benignCallDecode, BenignCallQMatMul,
	dispatchMutationByteSize, dispatchMutationDecode, DispatchMutationQMatMul,
	matchingMutationByteSize, matchingMutationDecode, MatchingMutationQMatMul,
	crossEnumDefaultByteSize, crossEnumDefaultDecode, CrossEnumDefaultQMatMul,
	pointerAliasMutationByteSize, pointerAliasMutationDecode, PointerAliasMutationQMatMul,
	holderAliasMutationByteSize, holderAliasMutationDecode, HolderAliasMutationQMatMul,
	mutatedSpecializedByteSize, mutatedSpecializedDecode, MutatedSpecializedQMatMul,
	unsafeAliasMutationByteSize, unsafeAliasMutationDecode, UnsafeAliasMutationQMatMul,
	uintptrAliasMutationByteSize, uintptrAliasMutationDecode, UintptrAliasMutationQMatMul,
	tupleAliasMutationByteSize, tupleAliasMutationDecode, TupleAliasMutationQMatMul,
	quantBlockByteSize, portableDequantize, QMatMul, cudaQMatMul,
	completeByteSize, completeDecode, CompleteQMatMul,
	singleByteSize, singleDecode, SingleQMatMul, unrelatedQMatMul,
	wrappedByteSize, wrappedDecode, WrappedQMatMul,
	rangeByteSize, rangeDecode, RangeQMatMul,
	staleByteSize, staleDecode, StaleQMatMul, unrelatedPolicy,
	metadataByteSize, metadataDecode, MetadataQMatMul,
	aliasByteSize, aliasDecode, AliasQMatMul,
	sharedEvidenceByteSize, sharedEvidenceDecode, SharedEvidenceQMatMul,
	suppressedAliasByteSize, suppressedAliasDecode, SuppressedAliasQMatMul,
	validatedByteSize, validatedDecode, ValidatedQMatMul,
	validatedThresholdByteSize, validatedThresholdDecode, ValidatedThresholdQMatMul,
	allValidatedMapByteSize, allValidatedMapDecode, AllValidatedMapQMatMul,
	mixedValidatedMapByteSize, mixedValidatedMapDecode, MixedValidatedMapQMatMul,
	globalTableByteSize, globalTableDecode, GlobalTableQMatMul,
	storageMapByteSize, storageMapDecode, StorageMapQMatMul,
	quantSize, sizeRootDecode, SizeRootQMatMul,
	prototypeSize, prototypeFormatDecode, PrototypeContractQMatMul,
	quantTypeSize, boundaryStorageDecode, BoundaryQMatMul,
	initMadeTableByteSize, initMadeTableDecode, InitMadeTableQMatMul,
	bareBlockInitByteSize, bareBlockInitDecode, BareBlockInitQMatMul,
	mutatedInitTableByteSize, mutatedInitTableDecode, MutatedInitTableQMatMul, mutateInitTableLater,
	completeInitTableByteSize, completeInitTableDecode, CompleteInitTableQMatMul,
	nilInitTableByteSize, nilInitTableDecode, NilInitTableQMatMul,
	rejectedInitTableByteSize, rejectedInitTableDecode,
	RejectedConditionalInitQMatMul, RejectedDuplicateInitQMatMul, RejectedDynamicInitQMatMul,
	RejectedAliasInitQMatMul, RejectedHelperInitQMatMul, RejectedExportedAliasInitQMatMul,
	RejectedConstantExitInitQMatMul, RecoveredNamedInitQMatMul, RecoveredIIFEInitQMatMul,
	RecoveredReturningInitQMatMul,
	singleCaseByteSize, singleCaseDecode, SingleCaseQMatMul,
	returnedByteSize, returnedDecode, ReturnedQMatMul,
	sharedGlobalByteSize, sharedGlobalDecode, SharedGlobalQMatMul,
	bridgedByteSize, bridgedDecode, BridgedQMatMul,
	negativeGuardByteSize, negativeGuardDecode, NegativeGuardQMatMul,
	dormantByteSize, dormantDecode, DormantQMatMul,
	handlerByteSize, handlerDecode, HandlerQMatMul,
	genericByteSize, genericDecode, GenericQMatMul,
	reachableByteSize, reachableDecode, ReachableQMatMul,
	stagedByteSize, stagedDecode, StagedQMatMul,
	outcomeByteSize, outcomeDecode, OutcomeQMatMul,
	directiveByteSize, directiveDecode, DirectiveQMatMul,
	taglessByteSize, taglessDecode, TaglessQMatMul,
	algebraByteSize, algebraDecode, AlgebraQMatMul,
	nilHandlerByteSize, nilHandlerDecode, NilHandlerQMatMul,
	errVariableByteSize, errVariableDecode, ErrVariableQMatMulOne, ErrVariableQMatMulTwo,
	namedResultPredicateByteSize, namedResultPredicateDecode,
	NamedResultPredicateQMatMulOne, NamedResultPredicateQMatMulTwo,
	namedRangeWriteByteSize, namedRangeWriteDecode,
	NamedRangeWriteQMatMulOne, NamedRangeWriteQMatMulTwo,
	emptyNamedRangeWriteByteSize, emptyNamedRangeWriteDecode,
	EmptyNamedRangeWriteQMatMulOne, EmptyNamedRangeWriteQMatMulTwo,
	multiResultSuccessByteSize, multiResultSuccessDecode,
	MultiResultSuccessQMatMulOne, MultiResultSuccessQMatMulTwo,
	forwardedTupleReturnByteSize, forwardedTupleReturnDecode,
	ForwardedTupleReturnQMatMulOne, ForwardedTupleReturnQMatMulTwo,
	tupleFallbackByteSize, tupleFallbackDecode,
	TupleFallbackQMatMulOne, TupleFallbackQMatMulTwo,
	tupleMixedReturnByteSize, tupleMixedReturnDecode,
	TupleMixedReturnQMatMulOne, TupleMixedReturnQMatMulTwo,
	defaultPredicateByteSize, defaultPredicateDecode, DefaultPredicateQMatMulOne, DefaultPredicateQMatMulTwo,
	localErrorAliasByteSize, localErrorAliasDecode, LocalErrorAliasQMatMulOne, LocalErrorAliasQMatMulTwo,
	fallthroughPredicateByteSize, fallthroughPredicateDecode,
	FallthroughPredicateQMatMulOne, FallthroughPredicateQMatMulTwo,
	breakContinuationByteSize, breakContinuationDecode,
	BreakContinuationQMatMulOne, BreakContinuationQMatMulTwo,
	rangeErrorMutationByteSize, rangeErrorMutationDecode,
	RangeErrorMutationQMatMulOne, RangeErrorMutationQMatMulTwo,
	staleNamedResultByteSize, staleNamedResultDecode,
	StaleNamedResultQMatMulOne, StaleNamedResultQMatMulTwo,
	defaultReachabilityByteSize, defaultReachabilityDecode,
	DefaultReachabilityQMatMulOne, DefaultReachabilityQMatMulTwo,
	fallthroughSourceExitByteSize, fallthroughSourceExitDecode,
	FallthroughSourceExitQMatMulOne, FallthroughSourceExitQMatMulTwo,
	namedResultPointerByteSize, namedResultPointerDecode,
	NamedResultPointerQMatMulOne, NamedResultPointerQMatMulTwo,
	crossReturnInnerByteSize, crossReturnInnerDecode, CrossReturnQMatMulOne, CrossReturnQMatMulTwo,
	namedErrorAssignmentByteSize, namedErrorAssignmentDecode,
	NamedErrorAssignmentQMatMulOne, NamedErrorAssignmentQMatMulTwo,
	taglessConditionalDefaultByteSize, taglessConditionalDefaultDecode,
	TaglessConditionalDefaultQMatMul, clauseOwnedReturnByteSize, clauseOwnedReturnDecode,
	ClauseOwnedReturnQMatMul, conditionalNamedErrorByteSize, conditionalNamedErrorDecode,
	ConditionalNamedErrorQMatMulOne, ConditionalNamedErrorQMatMulTwo,
	conditionalNamedBoolByteSize, conditionalNamedBoolDecode,
	ConditionalNamedBoolQMatMulOne, ConditionalNamedBoolQMatMulTwo,
	subjectIdentityByteSize, subjectIdentityDecode,
	SubjectIdentityQMatMulOne, SubjectIdentityQMatMulTwo,
	mixedShapeOrByteSize, mixedShapeOrDecode, MixedShapeOrQMatMul,
	implicitNilErrorByteSize, implicitNilErrorDecode,
	ImplicitNilErrorQMatMulOne, ImplicitNilErrorQMatMulTwo,
	transformedSubjectByteSize, transformedSubjectDecode,
	TransformedSubjectQMatMulOne, TransformedSubjectQMatMulTwo,
	partialNamedResultsByteSize, partialNamedResultsDecode,
	PartialNamedResultsQMatMulOne, PartialNamedResultsQMatMulTwo,
	siblingTransformByteSize, siblingTransformDecode, SiblingTransformQMatMul,
	compoundNamedResultByteSize, compoundNamedResultDecode,
	CompoundNamedResultQMatMulOne, CompoundNamedResultQMatMulTwo,
	elseReturnByteSize, elseReturnDecode, ElseReturnQMatMulOne, ElseReturnQMatMulTwo,
	alternativeUnionByteSize, alternativeUnionDecode,
	AlternativeUnionQMatMulOne, AlternativeUnionQMatMulTwo,
	nestedShapeAlternativesByteSize, nestedShapeAlternativesDecode,
	NestedShapeAlternativesQMatMulOne, NestedShapeAlternativesQMatMulTwo,
	transitiveShapeAlternativesByteSize, transitiveShapeAlternativesDecode,
	TransitiveShapeAlternativesQMatMulOne, TransitiveShapeAlternativesQMatMulTwo,
	literalShapeBoundaryByteSize, literalShapeBoundaryDecode,
	LiteralShapeBoundaryQMatMulOne, LiteralShapeBoundaryQMatMulTwo,
	shapeSwitchAlternativesByteSize, shapeSwitchAlternativesDecode,
	ShapeSwitchAlternativesQMatMul,
	capturedBindingByteSize, capturedBindingDecode,
	CapturedBindingQMatMulOne, CapturedBindingQMatMulTwo, CapturedBindingQMatMulThree,
	dominantErrorByteSize, dominantErrorDecode,
	DominantErrorQMatMulOne, DominantErrorQMatMulTwo, DominantErrorQMatMulThree,
	dominantGuardErrorByteSize, dominantGuardErrorDecode,
	DominantGuardErrorQMatMulOne, DominantGuardErrorQMatMulTwo,
	layerByteSize, decodeLayerPortableDecode, dequantizeLayerCPU, DecodeLayerQMatMul,
	rawSuppressedByteSize, rawSuppressedDecode, RawSuppressedQMatMul,
}

type TestFileRootQuant uint8

const (
	TestFileRootA TestFileRootQuant = iota
	TestFileRootB
	TestFileRootC
)

func testFileRootByteSize(quant TestFileRootQuant) int {
	switch quant {
	case TestFileRootA, TestFileRootB, TestFileRootC:
		return 16
	default:
		return 0
	}
}

func testFileRootPortableDecode(quant TestFileRootQuant) bool {
	switch quant {
	case TestFileRootA, TestFileRootB, TestFileRootC:
		return true
	default:
		return false
	}
}

func testFileRootQMatMulLayer(quant TestFileRootQuant) bool {
	switch quant {
	case TestFileRootA, TestFileRootB, TestFileRootC:
		return true
	default:
		return false
	}
}

func TestFileRootQMatMul(quant TestFileRootQuant) bool {
	return testFileRootQMatMulLayer(quant)
}

type MulMatRootQuant uint8

const (
	MulMatRootA MulMatRootQuant = iota
	MulMatRootB                 // want `quant variant MulMatRootB \(MulMatRootQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	MulMatRootC
)

func alternateRootByteSize(quant MulMatRootQuant) int {
	switch quant {
	case MulMatRootA, MulMatRootB, MulMatRootC:
		return 16
	default:
		return 0
	}
}

func alternateRootPortableDecode(quant MulMatRootQuant) bool {
	switch quant {
	case MulMatRootA, MulMatRootB, MulMatRootC:
		return true
	default:
		return false
	}
}

func alternateRootLayerOne(quant MulMatRootQuant) bool {
	switch quant {
	case MulMatRootA, MulMatRootC:
		return true
	default:
		return false
	}
}

func alternateRootLayerTwo(quant MulMatRootQuant) bool {
	switch quant {
	case MulMatRootA, MulMatRootC:
		return true
	default:
		return false
	}
}

func MulMat(quant MulMatRootQuant) bool {
	return alternateRootLayerOne(quant) && alternateRootLayerTwo(quant)
}

type StableFunctionVariableQuant uint8

const (
	StableFunctionVariableA StableFunctionVariableQuant = iota
	StableFunctionVariableB
	StableFunctionVariableC
)

func stableFunctionVariableByteSize(quant StableFunctionVariableQuant) int {
	switch quant {
	case StableFunctionVariableA, StableFunctionVariableB, StableFunctionVariableC:
		return 16
	default:
		return 0
	}
}

func stableFunctionVariablePortableDecode(quant StableFunctionVariableQuant) bool {
	switch quant {
	case StableFunctionVariableA, StableFunctionVariableB, StableFunctionVariableC:
		return true
	default:
		return false
	}
}

func stableFunctionVariableKernel() {}

func stableFunctionVariableGenericKernel[T any]() {}

func stableFunctionVariableGenericPairKernel[T, U any]() {}

func StableFunctionVariableQMatMul(quant StableFunctionVariableQuant) bool {
	kernel := stableFunctionVariableKernel
	kernelAlias := kernel
	genericKernel := stableFunctionVariableGenericKernel[int]
	genericPairKernel := stableFunctionVariableGenericPairKernel[int, string]
	first := map[StableFunctionVariableQuant]func(){
		StableFunctionVariableA: kernel,
		StableFunctionVariableB: genericKernel,
		StableFunctionVariableC: kernel,
	}
	if invoke := first[quant]; invoke != nil {
		invoke()
	}

	second := map[StableFunctionVariableQuant]func(){
		StableFunctionVariableA: kernelAlias,
		StableFunctionVariableB: genericPairKernel,
		StableFunctionVariableC: kernelAlias,
	}
	if invoke := second[quant]; invoke != nil {
		invoke()
		return true
	}
	return false
}

type UnsafeFunctionVariableQuant uint8

const (
	UnsafeFunctionVariableA UnsafeFunctionVariableQuant = iota
	UnsafeFunctionVariableB                             // want `quant variant UnsafeFunctionVariableB \(UnsafeFunctionVariableQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	UnsafeFunctionVariableC
)

var ExportedFunctionVariableKernel = stableFunctionVariableKernel

var exportedFunctionVariableTableOne = map[StableFunctionVariableQuant]func(){
	StableFunctionVariableA: ExportedFunctionVariableKernel,
	StableFunctionVariableB: ExportedFunctionVariableKernel,
	StableFunctionVariableC: ExportedFunctionVariableKernel,
}

var exportedFunctionVariableTableTwo = map[StableFunctionVariableQuant]func(){
	StableFunctionVariableA: ExportedFunctionVariableKernel,
	StableFunctionVariableB: ExportedFunctionVariableKernel,
	StableFunctionVariableC: ExportedFunctionVariableKernel,
}

func ExportedFunctionVariableTableQMatMul(quant StableFunctionVariableQuant) bool {
	if invoke := exportedFunctionVariableTableOne[quant]; invoke != nil {
		invoke()
	}
	if invoke := exportedFunctionVariableTableTwo[quant]; invoke != nil {
		invoke()
		return true
	}
	return false
}

func unsafeFunctionVariableByteSize(quant UnsafeFunctionVariableQuant) int {
	switch quant {
	case UnsafeFunctionVariableA, UnsafeFunctionVariableB, UnsafeFunctionVariableC:
		return 16
	default:
		return 0
	}
}

func unsafeFunctionVariablePortableDecode(quant UnsafeFunctionVariableQuant) bool {
	switch quant {
	case UnsafeFunctionVariableA, UnsafeFunctionVariableB, UnsafeFunctionVariableC:
		return true
	default:
		return false
	}
}

func UnsafeFunctionVariableQMatMul(quant UnsafeFunctionVariableQuant) bool {
	unstableKernel := stableFunctionVariableKernel
	unstableKernel = nil
	first := map[UnsafeFunctionVariableQuant]func(){
		UnsafeFunctionVariableA: stableFunctionVariableKernel,
		UnsafeFunctionVariableB: unstableKernel,
		UnsafeFunctionVariableC: stableFunctionVariableKernel,
	}
	if invoke := first[quant]; invoke != nil {
		invoke()
	}

	second := map[UnsafeFunctionVariableQuant]func(){
		UnsafeFunctionVariableA: stableFunctionVariableKernel,
		UnsafeFunctionVariableB: ExportedFunctionVariableKernel,
		UnsafeFunctionVariableC: stableFunctionVariableKernel,
	}
	if invoke := second[quant]; invoke != nil {
		invoke()
		return true
	}
	return false
}

type BackendMarkerTokenQuant uint8

const (
	BackendMarkerTokenA BackendMarkerTokenQuant = iota
	BackendMarkerTokenB                         // want `quant variant BackendMarkerTokenB \(BackendMarkerTokenQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	BackendMarkerTokenC
)

func backendMarkerTokenByteSize(quant BackendMarkerTokenQuant) int {
	switch quant {
	case BackendMarkerTokenA, BackendMarkerTokenB, BackendMarkerTokenC:
		return 16
	default:
		return 0
	}
}

func backendMarkerTokenPortableDecode(quant BackendMarkerTokenQuant) bool {
	switch quant {
	case BackendMarkerTokenA, BackendMarkerTokenB, BackendMarkerTokenC:
		return true
	default:
		return false
	}
}

func backendMarkerTokenPartialOne(quant BackendMarkerTokenQuant) bool {
	return quant == BackendMarkerTokenA || quant == BackendMarkerTokenC
}

func backendMarkerTokenPartialTwo(quant BackendMarkerTokenQuant) bool {
	return quant == BackendMarkerTokenA || quant == BackendMarkerTokenC
}

// "mps" is only a substring of "clamps", so this remains a CPU root.
func clampsQMatMul(quant BackendMarkerTokenQuant) bool {
	return backendMarkerTokenPartialOne(quant) && backendMarkerTokenPartialTwo(quant)
}

type IncidentalMatmulSubstringQuant uint8

const (
	IncidentalMatmulSubstringA IncidentalMatmulSubstringQuant = iota
	IncidentalMatmulSubstringB
	IncidentalMatmulSubstringC
)

func incidentalMatmulSubstringByteSize(quant IncidentalMatmulSubstringQuant) int {
	switch quant {
	case IncidentalMatmulSubstringA, IncidentalMatmulSubstringB, IncidentalMatmulSubstringC:
		return 16
	default:
		return 0
	}
}

func incidentalMatmulSubstringPortableDecode(quant IncidentalMatmulSubstringQuant) bool {
	switch quant {
	case IncidentalMatmulSubstringA, IncidentalMatmulSubstringB, IncidentalMatmulSubstringC:
		return true
	default:
		return false
	}
}

// These format-scaling helpers contain the letters "matmul" only across an
// identifier-word boundary and must not create a CPU matmul surface.
func FormatMultiplierOne(quant IncidentalMatmulSubstringQuant) bool {
	return quant == IncidentalMatmulSubstringA || quant == IncidentalMatmulSubstringC
}

func QuantFormatMultiplier(quant IncidentalMatmulSubstringQuant) bool {
	return quant == IncidentalMatmulSubstringA || quant == IncidentalMatmulSubstringC
}

type StandardErrorSentinelQuant uint8

const (
	StandardErrorSentinelA StandardErrorSentinelQuant = iota
	StandardErrorSentinelB                            // want `quant variant StandardErrorSentinelB \(StandardErrorSentinelQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	StandardErrorSentinelC
	StandardErrorSentinelD
	StandardErrorSentinelE
	StandardErrorSentinelF
	StandardErrorSentinelG
)

var standardErrorSentinelAddress = &os.ErrInvalid
var _ = func() { Canceled = nil }

func init() {
	io.ErrClosedPipe = nil
	standardErrorSentinelGenericMutation(0)
	standardErrorSentinelGenericPointerMutation(new(int))
	standardErrorSentinelGenericCaseMutation[int]()
}

func standardErrorSentinelGenericMutation[T any](value T) {
	switch any(value).(type) {
	case int:
		io.EOF = nil
	}
}

func standardErrorSentinelGenericPointerMutation[T any](value *T) {
	switch any(value).(type) {
	case *int:
		io.ErrUnexpectedEOF = nil
	}
}

func standardErrorSentinelGenericCaseMutation[T any]() {
	switch any(0).(type) {
	case T:
		io.ErrNoProgress = nil
	}
}

func standardErrorSentinelDeadMutation() {
	type callback func()
	dormant := func() { Canceled = nil }
	alias := dormant
	convertedAlias := callback(alias)
	_ = dormant
	_ = alias
	_ = convertedAlias
	_ = func() { Canceled = nil }
	_ = callback(func() { Canceled = nil })
	compared := func() { Canceled = nil }
	_ = compared == nil
	_ = []func(){compared}
	_ = struct{ hook func() }{hook: compared}
	_ = &struct{ hook func() }{hook: compared}
	var assigned func()
	assigned = func() { Canceled = nil }
	_ = assigned
	var overwritten func()
	overwritten = func() { Canceled = nil }
	overwritten = func() { Canceled = nil }
	_ = overwritten
	deadCall := func() { Canceled = nil }
	if false {
		deadCall()
	}
	nestedTarget := func() { Canceled = nil }
	nestedOuter := func() { nestedTarget() }
	_ = nestedTarget
	_ = nestedOuter
	var left, right func()
	left = func() { right() }
	right = func() {
		left()
		Canceled = nil
	}
	_ = left
	_ = right
	var installed func()
	installer := func() { installed = nestedTarget }
	_ = installer
	if installed != nil {
		installed()
	}
	if false {
		Canceled = nil
	}
	for false {
		Canceled = nil
	}
	for _, Canceled = range []error{} {
	}
	for Canceled = range (func(func(error) bool))(nil) {
	}
	if false {
		func() { Canceled = nil }()
	}
	switch any(0).(type) {
	case string:
		Canceled = nil
	}
	select {
	case (chan struct{})(nil) <- struct{}{}:
		Canceled = nil
	default:
	}
	_ = unsafe.Sizeof(func() { Canceled = nil })
	_ = len([1]func(){func() { Canceled = nil }})
}

func standardErrorSentinelByteSize(quant StandardErrorSentinelQuant) int {
	switch quant {
	case StandardErrorSentinelA, StandardErrorSentinelB, StandardErrorSentinelC, StandardErrorSentinelD, StandardErrorSentinelE, StandardErrorSentinelF, StandardErrorSentinelG:
		return 16
	default:
		return 0
	}
}

func standardErrorSentinelPortableDecode(quant StandardErrorSentinelQuant) bool {
	switch quant {
	case StandardErrorSentinelA, StandardErrorSentinelB, StandardErrorSentinelC, StandardErrorSentinelD, StandardErrorSentinelE, StandardErrorSentinelF, StandardErrorSentinelG:
		return true
	default:
		return false
	}
}

func standardErrorSentinelLayerOne(quant StandardErrorSentinelQuant) error {
	switch quant {
	case StandardErrorSentinelA:
		return nil
	case StandardErrorSentinelB:
		return errors.ErrUnsupported
	case StandardErrorSentinelC:
		return io.ErrClosedPipe
	case StandardErrorSentinelD:
		return os.ErrInvalid
	case StandardErrorSentinelE:
		return io.EOF
	case StandardErrorSentinelF:
		return io.ErrUnexpectedEOF
	case StandardErrorSentinelG:
		return io.ErrNoProgress
	default:
		return errors.ErrUnsupported
	}
}

func standardErrorSentinelLayerTwo(quant StandardErrorSentinelQuant) error {
	switch quant {
	case StandardErrorSentinelA:
		return nil
	case StandardErrorSentinelB:
		return Canceled
	case StandardErrorSentinelC:
		return io.ErrClosedPipe
	case StandardErrorSentinelD:
		return os.ErrInvalid
	case StandardErrorSentinelE:
		return io.EOF
	case StandardErrorSentinelF:
		return io.ErrUnexpectedEOF
	case StandardErrorSentinelG:
		return io.ErrNoProgress
	default:
		return errors.ErrUnsupported
	}
}

func StandardErrorSentinelQMatMul(quant StandardErrorSentinelQuant) error {
	if err := standardErrorSentinelLayerOne(quant); err != nil {
		return err
	}
	return standardErrorSentinelLayerTwo(quant)
}
