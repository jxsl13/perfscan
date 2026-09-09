package ps6084

import "unsafe"

//go:noescape
func blockLeaf(dst *float32, packed *byte, coefficients *float32)

//go:noescape
func wordLeaf(packed *uint16, coefficients *uint32)

//go:noescape
func scalarLeaf(packed uint64, coefficients *float32)

//go:noescape
func unsafeLeaf(packed, coefficients unsafe.Pointer)

type floatPointer *float32

//go:noescape
func namedPointerLeaf(packed *byte, coefficients floatPointer)

//go:noescape
func arrayPointerLeaf(packed *[8]byte, coefficients *[8]float32)

//go:noescape
func arrayValueLeaf(packed [8]byte, coefficients *[8]float32)

//go:noescape
func sliceLeaf(packed []byte, coefficients *float32)

//go:noescape
func narrowLeaf(packed uint32, coefficients *float32)

//go:noescape
func lengthScratchLeaf(packed *byte, coefficients *float32, count int)

type nativeKernel struct{}

type packedBlock struct {
	fields [8]byte
	scale  uint16
}

//go:noescape
func (nativeKernel) methodLeaf(packed *byte, coefficients []float64)

//go:noescape
func structLeaf(packed *packedBlock, coefficients *float32)

//go:noescape
func structValueLeaf(packed packedBlock, coefficients *float32)

func rangePlane(dst []float32, packed []byte, scale float32, table []float32) {
	var coefficients [8]float32
	for field := range coefficients { // want `fixed 8-element float32 scratch coefficients.*blockLeaf.*packed source packed`
		coefficients[field] = scale * table[packed[field]&15]
	}
	blockLeaf(&dst[0], &packed[0], &coefficients[0])
}

func classicPlane(packed []uint16, table []uint32) {
	coefficients := [4]uint32{}
	for field := 0; field < len(coefficients); field++ { // want `fixed 4-element uint32 scratch coefficients.*wordLeaf.*packed source packed`
		coefficients[field] = table[packed[field]&3]
	}
	wordLeaf(&packed[0], &coefficients[0])
}

func packedScalar(word uint64, table []float32) {
	var coefficients [8]float32
	for field := range coefficients { // want `fixed 8-element float32 scratch coefficients.*scalarLeaf.*packed source word`
		coefficients[field] = table[(word>>uint(field*4))&15]
	}
	scalarLeaf(word, &coefficients[0])
}

func unsafePointerCarrier(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients { // want `fixed 8-element float32 scratch coefficients.*unsafeLeaf.*packed source packed`
		coefficients[field] = table[packed[field]&15]
	}
	unsafeLeaf(unsafe.Pointer(&packed[0]), unsafe.Pointer(&coefficients[0]))
}

func namedPointerCarrier(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients { // want `fixed 8-element float32 scratch coefficients.*namedPointerLeaf.*packed source packed`
		coefficients[field] = table[packed[field]&15]
	}
	namedPointerLeaf(&packed[0], floatPointer(&coefficients[0]))
}

// Pointer-to-array arguments are common in runtime and crypto assembly APIs.
func toolchainArrayCarrier(packed *[8]byte, table *[16]float32) {
	var coefficients [8]float32
	for field := 0; field < 8; field++ { // want `fixed 8-element float32 scratch coefficients.*arrayPointerLeaf.*packed source packed`
		coefficients[field] = table[packed[field]&15]
	}
	arrayPointerLeaf(packed, &coefficients)
}

func fixedLengthArgument(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients { // want `fixed 8-element float32 scratch coefficients.*lengthScratchLeaf.*packed source packed`
		coefficients[field] = table[packed[field]&15]
	}
	lengthScratchLeaf(&packed[0], &coefficients[0], len(coefficients))
}

func integerRangeLength(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range len(coefficients) { // want `fixed 8-element float32 scratch coefficients.*blockLeaf.*packed source packed`
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func integerRangeMethod(kernel nativeKernel, packed []byte, table []float64) {
	var coefficients [4]float64
	for field := range 4 { // want `fixed 4-element float64 scratch coefficients.*methodLeaf.*packed source packed`
		coefficients[field] = table[packed[field]&3]
	}
	kernel.methodLeaf(&packed[0], coefficients[:])
}

func stableSourceAlias(packed []byte, table []float32) {
	alias := packed
	var coefficients [2]float32
	for field := range coefficients { // want `fixed 2-element float32 scratch coefficients.*blockLeaf.*packed source packed`
		coefficients[field] = table[alias[field]&1]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func stablePointerSourceAlias(packed *[8]byte, table []float32) {
	alias := packed
	var coefficients [8]float32
	for field := range coefficients { // want `fixed 8-element float32 scratch coefficients.*arrayPointerLeaf.*packed source packed`
		coefficients[field] = table[alias[field]&15]
	}
	arrayPointerLeaf(packed, &coefficients)
}

func fullSliceCarrier(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients { // want `fixed 8-element float32 scratch coefficients.*sliceLeaf.*packed source packed`
		coefficients[field] = table[packed[field]&15]
	}
	sliceLeaf(packed[:], &coefficients[0])
}

func offsetPointerCarrierStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[1], &coefficients[0])
}

func partialSliceCarrierStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	sliceLeaf(packed[1:], &coefficients[0])
}

func narrowedScalarCarrierStaysSilent(packed uint64, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[(packed>>uint(field*4))&15]
	}
	narrowLeaf(uint32(packed), &coefficients[0])
}

func arrayCopyDivergesStaysSilent(packed [8]byte, table []float32) {
	alias := packed
	packed[0] ^= 1
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[alias[field]&15]
	}
	arrayValueLeaf(packed, &coefficients)
}

func structCopyDivergesStaysSilent(block packedBlock, table []float32) {
	alias := block
	block.fields[0] ^= 1
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[alias.fields[field]&15]
	}
	structValueLeaf(block, &coefficients[0])
}

func repeatedBlocks(dst []float32, blocks [][]byte, table []float32) {
	for block := range blocks {
		packed := blocks[block]
		var coefficients [8]float32
		for field := range coefficients { // want `fixed 8-element float32 scratch coefficients.*blockLeaf.*packed source packed.*enclosing Go loop also repeats`
			coefficients[field] = table[packed[field]&15]
		}
		blockLeaf(&dst[0], &packed[0], &coefficients[0])
	}
}

func packedStructPlane(blocks []packedBlock, block int, table []float32) {
	var coefficients [8]float32
	for field := range coefficients { // want `fixed 8-element float32 scratch coefficients.*structLeaf.*packed source blocks`
		coefficients[field] = table[blocks[block].fields[field]&15]
	}
	structLeaf(&blocks[block], &coefficients[0])
}

func differentPackedStructStaysSilent(blocks []packedBlock, block int, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[blocks[block].fields[field]&15]
	}
	structLeaf(&blocks[0], &coefficients[0])
}

// A Go body is not native/noescape evidence.
func goLeaf(packed *byte, coefficients *float32) {}

func goBodyStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	goLeaf(&packed[0], &coefficients[0])
}

// A bodyless declaration without the compiler noescape contract is insufficient.
func opaqueLeaf(packed *byte, coefficients *float32)

func escapingLeafStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	opaqueLeaf(&packed[0], &coefficients[0])
}

// go:noescape
func spacedDirectiveLeaf(packed *byte, coefficients *float32)

func spacedDirectiveStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	spacedDirectiveLeaf(&packed[0], &coefficients[0])
}

func partialFillStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := 0; field < 7; field++ {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func stridedFillStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field*2] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func conditionalFillStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		if packed[field] != 0 {
			coefficients[field] = table[packed[field]&15]
		}
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func effectfulFillStaysSilent(packed []byte, coefficient func(byte) float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = coefficient(packed[field])
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func channelFillStaysSilent(packed []byte, values <-chan float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = <-values + float32(packed[field])
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func mapFillStaysSilent(packed []byte, table map[byte]float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func interveningWorkStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	observe(packed)
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func reusedScratchStaysSilent(packed []byte, table []float32) float32 {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
	return coefficients[0]
}

func reusedBeforeStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	observe(coefficients)
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func sourceNotPassedStaysSilent(table []float32) {
	var coefficients [8]float32
	var packed [8]byte
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, nil, &coefficients[0])
}

// Passing one decoded element is not evidence that the leaf receives the
// packed plane needed to reconstruct every coefficient.
//
//go:noescape
func elementLeaf(packed byte, coefficients *float32)

func sourceElementOnlyStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	elementLeaf(packed[0], &coefficients[0])
}

// An integer used only to address the packed plane is not itself packed
// metadata that lets the leaf reconstruct every coefficient.
//
//go:noescape
func offsetLeaf(offset int, coefficients *float32)

func sourceOffsetOnlyStaysSilent(packed []byte, offset int, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[offset+field]&15]
	}
	offsetLeaf(offset, &coefficients[0])
}

func lengthOnlyIsNotMetadata(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	lengthLeaf(len(packed), &coefficients[0])
}

//go:noescape
func lengthLeaf(length int, coefficients *float32)

func nonzeroInitializerStaysSilent(packed []byte, table []float32) {
	coefficients := [8]float32{1}
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func tooLargeStaysSilent(packed []byte, table []float32) {
	var coefficients [65]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func singleElementStaysSilent(packed []byte, table []float32) {
	var coefficients [1]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func functionAliasStaysSilent(packed []byte, table []float32) {
	leaf := blockLeaf
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	leaf(nil, &packed[0], &coefficients[0])
}

func transformedScratchStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[1])
}

// A uintptr does not carry the compiler's pointer noescape guarantee.
//
//go:noescape
func uintptrLeaf(packed *byte, coefficients uintptr)

func uintptrScratchStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	uintptrLeaf(&packed[0], uintptr(unsafe.Pointer(&coefficients[0])))
}

//go:noescape
func reinterpretedLeaf(packed *byte, coefficients *uint32)

func reinterpretedScratchStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	reinterpretedLeaf(&packed[0], (*uint32)(unsafe.Pointer(&coefficients[0])))
}

func multipleCallsStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	observe(blockLeafResult(&packed[0], &coefficients[0]))
}

//go:noescape
func blockLeafResult(packed *byte, coefficients *float32) int

//go:noescape
func conditionalLeaf(packed *byte, coefficients *float32) bool

func shortCircuitConsumerStaysSilent(enabled bool, packed []byte, table []float32) bool {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	return enabled && conditionalLeaf(&packed[0], &coefficients[0])
}

func assignedRangeIndexStaysSilent(packed []byte, table []float32) {
	var coefficients [8]float32
	var field int
	for field = range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func dynamicSliceStaysSilent(packed []byte, table []float32) {
	coefficients := make([]float32, 8)
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func closureRebindBreaksAlias(packed, other []byte, table []float32) {
	alias := packed
	func() { packed = other }()
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[alias[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func capturedScratchStaysSilent(packed []byte, table []float32) func() float32 {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
	return func() float32 { return coefficients[0] }
}

//perfscan:native-scratch-validated
func validatedWrapper(packed []byte, table []float32) {
	var coefficients [8]float32
	for field := range coefficients {
		coefficients[field] = table[packed[field]&15]
	}
	blockLeaf(nil, &packed[0], &coefficients[0])
}

func observe(...any) {}
