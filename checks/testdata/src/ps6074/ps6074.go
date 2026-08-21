package ps6074

func softmaxPipeline(values []float32) float32 { // want `softmaxPipeline runs ordered passes rowMaxF32 -> expSumF32 -> scaleRowF32 over the same slice; arm64 selects whole-pass scalar rowMaxF32, scaleRowF32 beside SIMD/assembly-backed expSumF32 while the scalar families are vectorized on amd64; repeated same-package hot-call fanout is 1.*maxNum/minNum or FMAXNM/FMINNM.*signed-zero ties bit-for-bit`
	maximum := rowMaxF32(values)
	sum := expSumF32(values, maximum)
	scaleRowF32(values, 1/sum)
	return maximum
}

func driveSoftmax(rows [][]float32) float32 {
	var result float32
	for _, row := range rows {
		result += softmaxPipeline(row)
	}
	return result
}

// All three non-amd64 variants are scalar, so this is missing vectorization,
// but not a partially vectorized target pipeline.
func allScalarPipeline(values []float32) float32 {
	maximum := allMaxF32(values)
	sum := allExpF32(values, maximum)
	allScaleF32(values, 1/sum)
	return maximum
}

func driveAllScalar(rows [][]float32) {
	for _, row := range rows {
		_ = allScalarPipeline(row)
	}
}

// The arm64 max body calls NEON and then has a scalar remainder loop. It is a
// vector implementation with a tail, not a whole-pass scalar fallback.
func tailPipeline(values []float32) float32 {
	maximum := tailMaxF32(values)
	sum := tailExpF32(values, maximum)
	tailScaleF32(values, 1/sum)
	return maximum
}

func driveTail(rows [][]float32) {
	for _, row := range rows {
		_ = tailPipeline(row)
	}
}

func differentSlices(left, right []float32) float32 {
	maximum := rowMaxF32(left)
	sum := expSumF32(left, maximum)
	scaleRowF32(right, 1/sum)
	return maximum
}

func twoStages(values []float32) float32 {
	maximum := rowMaxF32(values)
	return expSumF32(values, maximum)
}

// A neutral helper with no repeated same-package caller is not hot evidence.
func compose(values []float32) float32 {
	maximum := rowMaxF32(values)
	sum := expSumF32(values, maximum)
	scaleRowF32(values, 1/sum)
	return maximum
}

//perfscan:architecture-pipeline-validated full routed campaigns retained this layout.
func softmaxValidated(values []float32) float32 {
	maximum := rowMaxF32(values)
	sum := expSumF32(values, maximum)
	scaleRowF32(values, 1/sum)
	return maximum
}

func indirectStage(values []float32) float32 {
	maxFunction := rowMaxF32
	maximum := maxFunction(values)
	sum := expSumF32(values, maximum)
	scaleRowF32(values, 1/sum)
	return maximum
}

var _ = []any{
	driveSoftmax, driveAllScalar, driveTail, differentSlices, twoStages,
	compose, softmaxValidated, indirectStage,
}
