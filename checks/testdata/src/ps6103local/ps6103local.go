package ps6103local

func allocate(input []float64) []float64 { // want allocate:"fresh-reusable-result-allocation"
	result := make([]float64, len(input))
	return result
}

func consume([]float64) {}

func fixedMissingSeam(input []float64) {
	for step := 0; step < 5; step++ {
		result := allocate(input) // want `fixed 5-step loop repeatedly creates short-lived allocate result result; the package-local callee source-proves a fresh result allocation and has no compatible Into sibling`
		consume(result)
	}
}

func opaque(input []float64) []float64

func noSourceProof(input []float64) {
	for step := 0; step < 5; step++ {
		result := opaque(input)
		consume(result)
	}
}

func maybeAllocate(input []float64, enabled bool) []float64 {
	if enabled {
		return make([]float64, len(input))
	}
	return nil
}

func noAllPathsProof(input []float64) {
	for step := 0; step < 5; step++ {
		result := maybeAllocate(input, true)
		consume(result)
	}
}

func maybeStoredAllocation(input []float64, enabled bool) []float64 {
	var result []float64
	if enabled {
		result = make([]float64, len(input))
	}
	return result
}

func noStoredAllPathsProof(input []float64, enabled bool) {
	for step := 0; step < 5; step++ {
		result := maybeStoredAllocation(input, enabled)
		consume(result)
	}
}

func maybeNamedAllocation(input []float64, enabled bool) (result []float64) {
	if enabled {
		result = make([]float64, len(input))
	}
	return result
}

func noNamedAllPathsProof(input []float64, enabled bool) {
	for step := 0; step < 5; step++ {
		result := maybeNamedAllocation(input, enabled)
		consume(result)
	}
}

func separatedAllocation(input []float64) []float64 { // want separatedAllocation:"fresh-reusable-result-allocation"
	var result []float64
	result = make([]float64, len(input))
	return result
}

func fixedSeparatedAllocation(input []float64) {
	for step := 0; step < 5; step++ {
		result := separatedAllocation(input) // want `fixed 5-step loop repeatedly creates short-lived separatedAllocation result result; the package-local callee source-proves a fresh result allocation`
		consume(result)
	}
}

func branchAllocation(input []float64) []float64 { // want branchAllocation:"fresh-reusable-result-allocation"
	var result []float64
	if len(input) > 0 {
		result = make([]float64, len(input))
	} else {
		result = make([]float64, 1)
	}
	return result
}

func fixedBranchAllocation(input []float64) {
	for step := 0; step < 5; step++ {
		result := branchAllocation(input) // want `fixed 5-step loop repeatedly creates short-lived branchAllocation result result; the package-local callee source-proves a fresh result allocation`
		consume(result)
	}
}

func rebind(result *[]float64) {
	*result = nil
}

func addressRebound(input []float64) []float64 {
	result := make([]float64, len(input))
	rebind(&result)
	return result
}

func noAddressEscapeProof(input []float64) {
	for step := 0; step < 5; step++ {
		result := addressRebound(input)
		consume(result)
	}
}

func closureRebound(input []float64) []float64 {
	result := make([]float64, len(input))
	func() { result = nil }()
	return result
}

func noClosureEscapeProof(input []float64) {
	for step := 0; step < 5; step++ {
		result := closureRebound(input)
		consume(result)
	}
}

func setLoopStep(step *int) {
	*step = 5
}

func noInductionWrite(input []float64) {
	for step := 0; step < 5; step++ {
		result := allocate(input)
		consume(result)
		step = 5
	}
}

func noInductionAddressExposure(input []float64) {
	for step := 0; step < 5; step++ {
		result := allocate(input)
		consume(result)
		setLoopStep(&step)
	}
}

func fixedRangeIndexWrite(input []float64) {
	for step := range [5]struct{}{} {
		result := allocate(input) // want `fixed 5-step loop repeatedly creates short-lived allocate result result; the package-local callee source-proves a fresh result allocation`
		consume(result)
		step++
	}
}

func allocateCount(count int) []float64 { // want allocateCount:"fresh-reusable-result-allocation"
	return make([]float64, count)
}

func fixedCountShape(count int) {
	for step := 0; step < 5; step++ {
		result := allocateCount(count) // want `fixed 5-step loop repeatedly creates short-lived allocateCount result result; the package-local callee source-proves a fresh result allocation`
		consume(result)
	}
}

func noChangingCountShape(count int) {
	for step := 0; step < 5; step++ {
		result := allocateCount(count)
		consume(result)
		count++
	}
}

func noAddressExposedCountShape(count int) {
	for step := 0; step < 5; step++ {
		result := allocateCount(count)
		consume(result)
		setLoopStep(&count)
	}
}

var retainedResult []float64

func noConversionStore(input []float64) {
	for step := 0; step < 5; step++ {
		result := allocate(input)
		retainedResult = []float64(result)
	}
}

func fixedConversionConsumer(input []float64) {
	for step := 0; step < 5; step++ {
		result := allocate(input) // want `fixed 5-step loop repeatedly creates short-lived allocate result result; the package-local callee source-proves a fresh result allocation`
		consume([]float64(result))
	}
}

type reusableBuffer []float64

var cachedBuffer reusableBuffer

func (buffer *reusableBuffer) replace() {
	*buffer = cachedBuffer
}

func replacedAllocation(count int) reusableBuffer {
	result := make(reusableBuffer, count)
	result.replace()
	return result
}

func noPointerReceiverProof(count int) {
	for step := 0; step < 5; step++ {
		result := replacedAllocation(count)
		consume(result)
	}
}

func storedReplacementAllocation(count int) reusableBuffer {
	result := make(reusableBuffer, count)
	replace := result.replace
	replace()
	return result
}

func noStoredPointerReceiverProof(count int) {
	for step := 0; step < 5; step++ {
		result := storedReplacementAllocation(count)
		consume(result)
	}
}

func (buffer reusableBuffer) observe() {}

func observedAllocation(count int) reusableBuffer { // want observedAllocation:"fresh-reusable-result-allocation"
	result := make(reusableBuffer, count)
	result.observe()
	return result
}

func fixedValueReceiverProof(count int) {
	for step := 0; step < 5; step++ {
		result := observedAllocation(count) // want `fixed 5-step loop repeatedly creates short-lived observedAllocation result result; the package-local callee source-proves a fresh result allocation`
		consume(result)
	}
}

func noParenthesizedInductionWrite(input []float64) {
	for step := 0; step < 5; step++ {
		result := allocate(input)
		consume(result)
		(step) = 5
	}
}

func noParenthesizedShapeWrite(count int) {
	for range 5 {
		result := allocateCount(count)
		consume(result)
		(count)++
	}
}

func fixedShapeIndexRead(count int) {
	outputs := make([]int, count+1)
	for range 5 {
		result := allocateCount(count) // want `fixed 5-step loop repeatedly creates short-lived allocateCount result result; the package-local callee source-proves a fresh result allocation`
		consume(result)
		outputs[count] = 1
	}
}

func noPreLoopShapeAlias(count int) {
	alias := &count
	for range 5 {
		result := allocateCount(count)
		consume(result)
		(*alias)++
	}
}

func noPreLoopInductionAlias(input []float64) {
	step := 0
	alias := &step
	for step = 0; step < 5; step++ {
		result := allocate(input)
		consume(result)
		*alias = 5
	}
}

func initializedFixedShape() {
	count := 8
	for range 5 {
		result := allocateCount(count) // want `fixed 5-step loop repeatedly creates short-lived allocateCount result result; the package-local callee source-proves a fresh result allocation`
		consume(result)
	}
}
