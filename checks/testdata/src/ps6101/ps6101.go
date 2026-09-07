package ps6101

import (
	fakerand "fakerand"
	"math"
	"math/rand"
	"testing"
)

func BenchmarkMoECombineNormWeights(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = rng.NormFloat64()
	}
	for i := 0; i < b.N; i++ {
		moeCombine(weights)
	}
}

func moeCombine(weights []float64) float64 {
	denom := 0.0
	for i := range weights {
		denom += weights[i]
	}
	if denom > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		return denom
	}
	return 0
}

func BenchmarkHelperCallGraphCenteredProbabilities(b *testing.B) {
	probabilities := makeCenteredProbabilities()
	for i := 0; i < b.N; i++ {
		gatedProbabilityKernel(probabilities, 0.25)
	}
}

func makeCenteredProbabilities() []float64 {
	rng := rand.New(rand.NewSource(2))
	probabilities := make([]float64, 64)
	for i := range probabilities {
		probabilities[i] = rng.Float64() - 0.5
	}
	return probabilities
}

func gatedProbabilityKernel(probabilities []float64, threshold float64) float64 {
	total := 0.0
	for _, probability := range probabilities {
		total += probability
	}
	if total >= threshold { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		return total
	}
	return 0
}

func BenchmarkNormFloat32Gates(b *testing.B) {
	rng := rand.New(rand.NewSource(3))
	gateWeights := make([]float32, 16)
	for i := range gateWeights {
		gateWeights[i] = float32(rng.NormFloat64())
	}
	for i := 0; i < b.N; i++ {
		denom := float32(0)
		for _, gateWeight := range gateWeights {
			denom += gateWeight
		}
		if denom != 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			_ = denom
		}
	}
}

func BenchmarkAppendScaleInputs(b *testing.B) {
	rng := rand.New(rand.NewSource(4))
	var scales []float64
	for i := 0; i < 32; i++ {
		scales = append(scales, rng.NormFloat64())
	}
	for i := 0; i < b.N; i++ {
		scaleSum := 0.0
		for _, scale := range scales {
			scaleSum += scale
		}
		if scaleSum < 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			_ = scaleSum
		}
	}
}

func BenchmarkPositiveFixtureSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(5))
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = math.Abs(rng.NormFloat64()) + 0.125
	}
	for i := 0; i < b.N; i++ {
		moeCombine(weights)
	}
}

func BenchmarkExplicitAssertionSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(6))
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = rng.NormFloat64()
	}
	denom := 0.0
	for _, weight := range weights {
		denom += weight
	}
	if denom <= 0 {
		b.Fatal("hot branch not reached")
	}
	if denom > 0 {
		_ = denom
	}
}

func BenchmarkBranchCounterSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(7))
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = rng.NormFloat64()
	}
	hotBranches := 0
	for i := 0; i < b.N; i++ {
		denom := 0.0
		for _, weight := range weights {
			denom += weight
		}
		if denom > 0 {
			hotBranches++
		}
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not reached")
	}
}

func BenchmarkUnrelatedSymmetricRandomSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(8))
	samples := make([]float64, 32)
	for i := range samples {
		samples[i] = rng.NormFloat64()
	}
	for i := 0; i < b.N; i++ {
		total := 0.0
		for _, sample := range samples {
			total += sample
		}
		if total > 0 {
			_ = total
		}
	}
}

func BenchmarkRandomWithoutAggregateGateSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(9))
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = rng.NormFloat64()
	}
	for i := 0; i < b.N; i++ {
		if len(weights) > 0 {
			_ = weights[0]
		}
	}
}

func helperOutsideBenchmark() {
	rng := rand.New(rand.NewSource(10))
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = rng.NormFloat64()
	}
	moeCombine(weights)
}

type benchmarkConfig struct {
	GateWeights []float64
}

func BenchmarkConfigFieldGateWeights(b *testing.B) {
	rng := rand.New(rand.NewSource(11))
	cfg := benchmarkConfig{GateWeights: make([]float64, 32)}
	for i := range cfg.GateWeights {
		cfg.GateWeights[i] = rng.NormFloat64()
	}
	for i := 0; i < b.N; i++ {
		configFieldKernel(cfg)
	}
}

func configFieldKernel(cfg benchmarkConfig) float64 {
	denom := 0.0
	for _, weight := range cfg.GateWeights {
		denom += weight
	}
	if denom > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		return denom
	}
	return 0
}

func BenchmarkCenteredSubtractOtherSide(b *testing.B) {
	rng := rand.New(rand.NewSource(12))
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = 0.5 - rng.Float64()
	}
	for i := 0; i < b.N; i++ {
		centeredOtherSideKernel(weights)
	}
}

func centeredOtherSideKernel(weights []float64) float64 {
	denom := 0.0
	for _, weight := range weights {
		denom += weight
	}
	if denom > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		return denom
	}
	return 0
}

type fakeMath struct{}

func (fakeMath) Abs(float64) float64 { return -1 }

func BenchmarkShadowedAbsStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(13))
	math := fakeMath{}
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = math.Abs(rng.NormFloat64()) + 0.125
	}
	for i := 0; i < b.N; i++ {
		shadowedAbsKernel(weights)
	}
}

func shadowedAbsKernel(weights []float64) float64 {
	denom := 0.0
	for _, weight := range weights {
		denom += weight
	}
	if denom > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		return denom
	}
	return 0
}

func BenchmarkUnrelatedAssertionStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(14))
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = rng.NormFloat64()
	}
	guard := 1
	if guard <= 0 {
		b.Fatal("unrelated setup failed")
	}
	for i := 0; i < b.N; i++ {
		unrelatedAssertionKernel(weights)
	}
}

func unrelatedAssertionKernel(weights []float64) float64 {
	denom := 0.0
	for _, weight := range weights {
		denom += weight
	}
	if denom > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		return denom
	}
	return 0
}

func BenchmarkUnrelatedBranchCounterStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(15))
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = rng.NormFloat64()
	}
	hotBranches := 0
	for i := 0; i < b.N; i++ {
		if len(weights) > 0 {
			hotBranches++
		}
		unrelatedBranchCounterKernel(weights)
	}
	if hotBranches == 0 {
		b.Fatal("unrelated branch not reached")
	}
}

func unrelatedBranchCounterKernel(weights []float64) float64 {
	denom := 0.0
	for _, weight := range weights {
		denom += weight
	}
	if denom > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		return denom
	}
	return 0
}

type fakeRandom struct{}

func (fakeRandom) NormFloat64() float64 { return -1 }

func BenchmarkLookalikeRandomMethodSilent(b *testing.B) {
	rng := fakeRandom{}
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = rng.NormFloat64()
	}
	for i := 0; i < b.N; i++ {
		denom := 0.0
		for _, weight := range weights {
			denom += weight
		}
		if denom > 0 {
			_ = denom
		}
	}
}

func BenchmarkUncenteredUniformSubtractionSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(16))
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = rng.Float64() - 0.125
	}
	for i := 0; i < b.N; i++ {
		denom := 0.0
		for _, weight := range weights {
			denom += weight
		}
		if denom > 0 {
			_ = denom
		}
	}
}

type fakeLogger struct{}

func (fakeLogger) Fatal(string) {}

func BenchmarkLookalikeFatalStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(17))
	weights := make([]float64, 32)
	for i := range weights {
		weights[i] = rng.NormFloat64()
	}
	denom := 0.0
	for _, weight := range weights {
		denom += weight
	}
	logger := fakeLogger{}
	if denom <= 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		logger.Fatal("hot branch not reached")
	}
	for i := 0; i < b.N; i++ {
		if denom > 0 {
			_ = denom
		}
	}
}

func BenchmarkIndependentProofDoesNotSuppressOtherGate(b *testing.B) {
	rng := rand.New(rand.NewSource(18))
	weights := []float64{rng.NormFloat64()}
	scales := []float64{rng.NormFloat64()}
	weightSum := weights[0]
	scaleSum := scales[0]
	if scaleSum <= 0 {
		b.Fatal("scale branch not reached")
	}
	if weightSum > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		_ = weightSum
	}
}

func BenchmarkWrongCounterPolarityStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(19))
	weights := []float64{rng.NormFloat64()}
	hotBranches := 0
	denom := weights[0]
	if denom > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		hotBranches++
	}
	if hotBranches > 0 {
		b.Fatal("inverted proof")
	}
}

func BenchmarkOverwrittenInputSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(20))
	weights := []float64{rng.NormFloat64()}
	weights[0] = 1
	denom := weights[0]
	if denom > 0 {
		_ = denom
	}
}

func BenchmarkBranchOverwriteConservativeSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(201))
	weights := []float64{rng.NormFloat64()}
	if b.N > 1 {
		weights[0] = 1
	}
	denom := weights[0]
	if denom > 0 {
		_ = denom
	}
}

func BenchmarkLaterRandomAssignmentSilent(b *testing.B) {
	weights := []float64{1}
	denom := weights[0]
	if denom > 0 {
		_ = denom
	}
	rng := rand.New(rand.NewSource(202))
	weights[0] = rng.NormFloat64()
}

func BenchmarkShadowedInputSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(21))
	weights := []float64{rng.NormFloat64()}
	_ = weights
	{
		weights := []float64{1}
		denom := weights[0]
		if denom > 0 {
			_ = denom
		}
	}
}

func BenchmarkDistinctFieldInstancesSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(22))
	randomConfig := benchmarkConfig{GateWeights: []float64{rng.NormFloat64()}}
	positiveConfig := benchmarkConfig{GateWeights: []float64{1}}
	_ = randomConfig
	denom := positiveConfig.GateWeights[0]
	if denom > 0 {
		_ = denom
	}
}

func BenchmarkCompositeFieldInputStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(221))
	cfg := benchmarkConfig{GateWeights: []float64{rng.NormFloat64()}}
	denom := cfg.GateWeights[0]
	if denom > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		_ = denom
	}
}

func makeValuesWithUnrelatedName() []float64 {
	rng := rand.New(rand.NewSource(23))
	values := []float64{rng.NormFloat64()}
	return values
}

func renamedKernel(xs []float64) float64 {
	total := xs[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		return total
	}
	return 0
}

func BenchmarkRenamedHelperFlow(b *testing.B) {
	weights := makeValuesWithUnrelatedName()
	_ = renamedKernel(weights)
}

type methodKernel struct{}

func (*methodKernel) run(xs []float64) float64 {
	total := xs[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		return total
	}
	return 0
}

func BenchmarkMethodHelperFlow(b *testing.B) {
	rng := rand.New(rand.NewSource(24))
	weights := []float64{rng.NormFloat64()}
	worker := &methodKernel{}
	_ = worker.run(weights)
}

func BenchmarkUncalledClosureSilent(b *testing.B) {
	_ = func() {
		rng := rand.New(rand.NewSource(25))
		weights := []float64{rng.NormFloat64()}
		denom := weights[0]
		if denom > 0 {
			_ = denom
		}
	}
}

func BenchmarkUncalledFailureClosureStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(26))
	weights := []float64{rng.NormFloat64()}
	denom := weights[0]
	if denom <= 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		_ = func() { b.Fatal("not called") }
	}
	if denom > 0 {
		_ = denom
	}
}

func BenchmarkAliasedInputStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(27))
	weights := []float64{rng.NormFloat64()}
	alias := weights
	total := alias[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		_ = total
	}
}

func BenchmarkAliasOverwriteSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(271))
	weights := []float64{rng.NormFloat64()}
	alias := weights
	alias[0] = 1
	denom := weights[0]
	if denom > 0 {
		_ = denom
	}
}

type aggregateStats struct{ Sum float64 }

func BenchmarkFieldAggregateStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(28))
	weights := []float64{rng.NormFloat64()}
	stats := aggregateStats{}
	stats.Sum += weights[0]
	if stats.Sum > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		_ = stats.Sum
	}
}

func BenchmarkSquaredInputSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(29))
	weights := []float64{rng.NormFloat64()}
	total := weights[0] * weights[0]
	if total > 0 {
		_ = total
	}
}

func BenchmarkAbsAggregateSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(30))
	weights := []float64{rng.NormFloat64()}
	total := math.Abs(weights[0])
	if total > 0 {
		_ = total
	}
}

func BenchmarkNamedConstantCompoundGate(b *testing.B) {
	const zero = 0.0
	rng := rand.New(rand.NewSource(31))
	weights := []float64{rng.NormFloat64()}
	denom := weights[0]
	enabled := b.N > 0
	if enabled && denom > zero { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		_ = denom
	}
}

func BenchmarkMismatchedThresholdProofStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(32))
	weights := []float64{rng.NormFloat64()}
	denom := weights[0]
	if denom == 42 {
		b.Fatal("unrelated threshold")
	}
	if denom > 100 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		_ = denom
	}
}

func BenchmarkProofBeforeSameSourceReassignmentStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(320))
	weights := []float64{rng.NormFloat64()}
	denom := weights[0]
	if denom <= 0 {
		b.Fatal("first value is positive")
	}
	denom = -weights[0]
	if denom > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		_ = denom
	}
}

func BenchmarkConditionalProofDoesNotSuppress(b *testing.B) {
	rng := rand.New(rand.NewSource(321))
	weights := []float64{rng.NormFloat64()}
	denom := weights[0]
	if b.N > 1 {
		if denom <= 0 {
			b.Fatal("conditional proof")
		}
	}
	if denom > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		_ = denom
	}
}

func BenchmarkCorrectNegativeGateProofSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(33))
	weights := []float64{rng.NormFloat64()}
	denom := weights[0]
	if denom >= 0 {
		b.Fatal("negative branch not reached")
	}
	if denom < 0 {
		_ = denom
	}
}

func BenchmarkNamedStrictPositiveFixtureSilent(b *testing.B) {
	const epsilon = 0.125
	rng := rand.New(rand.NewSource(34))
	weights := []float64{math.Abs(rng.NormFloat64()) + epsilon}
	denom := weights[0]
	if denom > 0 {
		_ = denom
	}
}

func BenchmarkFakeRandPackageSilent(b *testing.B) {
	rng := fakerand.Rand{}
	weights := []float64{rng.NormFloat64()}
	denom := weights[0]
	if denom > 0 {
		_ = denom
	}
}

func BenchmarkNotActuallyATestingBenchmark() {
	rng := rand.New(rand.NewSource(35))
	weights := []float64{rng.NormFloat64()}
	denom := weights[0]
	if denom > 0 {
		_ = denom
	}
}

var reviewSinkFloat float64

func BenchmarkReviewNonnegativeMaxShiftSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(101))
	weights := []float64{rng.NormFloat64() + math.MaxFloat64}
	total := weights[0]
	if total >= 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkReviewUnsignedRoundTripSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(102))
	weights := []float64{float64(uint64(rng.NormFloat64()))}
	total := weights[0]
	if total >= 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkReviewUntimedSetupGateSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(103))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	if total > 0 {
		reviewSinkFloat = total
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reviewSinkFloat = float64(i)
	}
}

func BenchmarkReviewSubbenchmarkClosure(b *testing.B) {
	b.Run("signed", func(b *testing.B) {
		rng := rand.New(rand.NewSource(104))
		weights := []float64{rng.NormFloat64()}
		for i := 0; i < b.N; i++ {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				reviewSinkFloat = total
			}
		}
	})
	b.ResetTimer()
}

func BenchmarkReviewOrWithAlwaysTrueArmSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(105))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	always := true
	if always || total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkReviewPointerOverwriteSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(106))
	weights := []float64{rng.NormFloat64()}
	pointer := &weights
	(*pointer)[0] = 1
	total := weights[0]
	if total > 0 {
		reviewSinkFloat = total
	}
}

func reviewRequirePositive(b *testing.B, total float64) {
	if total <= 0 {
		b.Fatal("hot branch not reached")
	}
}

func BenchmarkReviewHelperAssertionSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(107))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	reviewRequirePositive(b, total)
	if total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkReviewFailAssertionSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(108))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	if total <= 0 {
		b.Fail()
	}
	if total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkReviewFatalElseAssertionSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(109))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	if total > 0 {
		reviewSinkFloat = total
	} else {
		b.Fatal("hot branch not reached")
	}
}

func BenchmarkReviewRecoveredPanicProof(b *testing.B) {
	defer func() { _ = recover() }()
	rng := rand.New(rand.NewSource(110))
	weights := []float64{rng.NormFloat64()}
	total := weights[0] - weights[0]
	if total <= 0 {
		panic("hot branch not reached")
	}
	if total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkReviewVacuousCounterProof(b *testing.B) {
	rng := rand.New(rand.NewSource(111))
	weights := []float64{rng.NormFloat64()}
	total := weights[0] - weights[0]
	hotBranches := 0
	if total > 0 {
		hotBranches++
		reviewSinkFloat = total
	}
	hotBranches = 1
	if hotBranches == 0 {
		b.Fatal("hot branch not reached")
	}
}

func BenchmarkReviewWriteAfterBreak(b *testing.B) {
	rng := rand.New(rand.NewSource(112))
	weights := []float64{rng.NormFloat64()}
	for {
		break
		weights[0] = 1
	}
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func BenchmarkReviewZeroIterationOverwrite(b *testing.B) {
	rng := rand.New(rand.NewSource(113))
	weights := []float64{rng.NormFloat64()}
	count := 0
	for i := 0; i < count; i++ {
		weights[0] = 1
	}
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func reviewMakeWeightsPair() ([]float64, error) {
	rng := rand.New(rand.NewSource(114))
	return []float64{rng.NormFloat64()}, nil
}

func BenchmarkReviewValueErrorHelper(b *testing.B) {
	weights, err := reviewMakeWeightsPair()
	if err != nil {
		b.Fatal(err)
	}
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

type reviewDynamicLimits struct {
	Threshold float64
}

func BenchmarkReviewDynamicFieldThreshold(b *testing.B) {
	rng := rand.New(rand.NewSource(115))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	limits := reviewDynamicLimits{Threshold: float64(b.N)}
	if total > limits.Threshold { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

type reviewPointerKernel struct {
	GateWeights []float64
}

func (kernel *reviewPointerKernel) run() {
	total := kernel.GateWeights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func BenchmarkReviewPointerReceiverMethod(b *testing.B) {
	rng := rand.New(rand.NewSource(116))
	kernel := &reviewPointerKernel{GateWeights: []float64{rng.NormFloat64()}}
	kernel.run()
}

func BenchmarkReviewGateAfterResetStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(117))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	b.ResetTimer()
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func BenchmarkReviewStopStartTimerStillReports(b *testing.B) {
	rng := rand.New(rand.NewSource(118))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	b.StopTimer()
	if total > 0 {
		reviewSinkFloat = total
	}
	b.StartTimer()
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func reviewRequirePositiveAlias(b *testing.B, total float64) {
	checkedTotal := total
	if checkedTotal <= 0 {
		b.Fatal("hot branch not reached")
	}
}

func BenchmarkReviewHelperAssertionAliasSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(119))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	reviewRequirePositiveAlias(b, total)
	if total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkReviewSubbenchmarkOuterProofSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(120))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	b.Run("signed", func(b *testing.B) {
		if total > 0 {
			reviewSinkFloat = total
		}
	})
	if total <= 0 {
		b.Fatal("hot branch not reached")
	}
}

func BenchmarkFinalLoopSetupGate(b *testing.B) {
	rng := rand.New(rand.NewSource(201))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	if total > 0 {
		reviewSinkFloat = total
	}
	for b.Loop() {
		reviewSinkFloat = total
	}
}

func BenchmarkFinalLoopCleanupGate(b *testing.B) {
	rng := rand.New(rand.NewSource(202))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	for b.Loop() {
		reviewSinkFloat = total
	}
	if total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkFinalConditionalReset(b *testing.B) {
	rng := rand.New(rand.NewSource(203))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
	if b.N < 0 {
		b.ResetTimer()
	}
}

func finalNamedSubbenchmark(b *testing.B) {
	rng := rand.New(rand.NewSource(204))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func BenchmarkFinalNamedSubbenchmark(b *testing.B) {
	b.Run("signed", finalNamedSubbenchmark)
}

func BenchmarkFinalSecondIterationOverwrite(b *testing.B) {
	rng := rand.New(rand.NewSource(205))
	weights := []float64{rng.NormFloat64()}
	for i := 0; i < 2; i++ {
		if i == 1 {
			weights[0] = 1
		}
	}
	total := weights[0]
	if total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkFinalSecondIterationRandom(b *testing.B) {
	rng := rand.New(rand.NewSource(206))
	weights := []float64{1}
	for i := 0; i < 2; i++ {
		if i == 1 {
			weights[0] = rng.NormFloat64()
		}
	}
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func BenchmarkFinalNonDominatingFatal(b *testing.B) {
	rng := rand.New(rand.NewSource(207))
	weights := []float64{rng.NormFloat64()}
	total := weights[0] - weights[0]
	if total <= 0 {
		if true {
			return
		}
		b.Fatal("not reached")
	}
	if total > 0 {
		reviewSinkFloat = total
	}
}

func finalNamedReturn() (weights []float64) {
	rng := rand.New(rand.NewSource(208))
	weights = []float64{rng.NormFloat64()}
	return
}

func BenchmarkFinalNamedReturn(b *testing.B) {
	weights := finalNamedReturn()
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func BenchmarkFinalSubbenchmarkCounter(b *testing.B) {
	rng := rand.New(rand.NewSource(209))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	hotBranches := 0
	b.Run("signed", func(b *testing.B) {
		if total > 0 {
			hotBranches++
			reviewSinkFloat = total
		}
	})
	if hotBranches == 0 {
		b.Fatal("hot branch not reached")
	}
}

func finalTotal(weights []float64) float64 {
	total := weights[0]
	return total
}

func BenchmarkFinalDirectHelperGate(b *testing.B) {
	rng := rand.New(rand.NewSource(210))
	weights := []float64{rng.NormFloat64()}
	if finalTotal(weights) > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = weights[0]
	}
}

func BenchmarkFinalZeroAddProof(b *testing.B) {
	rng := rand.New(rand.NewSource(211))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	checkedTotal := total + 0
	if checkedTotal <= 0 {
		b.Fatal("hot branch not reached")
	}
	if total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkFinalOuterGateWithRun(b *testing.B) {
	rng := rand.New(rand.NewSource(212))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	if total > 0 {
		reviewSinkFloat = total
	}
	b.Run("control", func(b *testing.B) {
		for b.Loop() {
			reviewSinkFloat = 1
		}
	})
}

func BenchmarkFinalLaterRecoverDefer(b *testing.B) {
	rng := rand.New(rand.NewSource(213))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	if total <= 0 {
		panic("hot branch not reached")
	}
	defer func() { _ = recover() }()
	if total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkFinalOtherIndexOverwrite(b *testing.B) {
	rng := rand.New(rand.NewSource(214))
	weights := []float64{rng.NormFloat64(), rng.NormFloat64()}
	weights[1] = 1
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func BenchmarkFinalThresholdZeroAddProof(b *testing.B) {
	rng := rand.New(rand.NewSource(215))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	threshold := float64(b.N)
	checkedThreshold := threshold + 0
	if total <= checkedThreshold {
		b.Fatal("hot branch not reached")
	}
	if total > threshold {
		reviewSinkFloat = total
	}
}

func BenchmarkFinalNegatedComparator(b *testing.B) {
	rng := rand.New(rand.NewSource(216))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	if !(total <= 0) { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

type finalNestedInput struct {
	GateWeights []float64
}

type finalNestedConfig struct {
	Input finalNestedInput
}

func BenchmarkFinalNestedConfigField(b *testing.B) {
	rng := rand.New(rand.NewSource(217))
	config := finalNestedConfig{Input: finalNestedInput{GateWeights: []float64{rng.NormFloat64()}}}
	total := config.Input.GateWeights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func finalAsyncGate(weights []float64, done chan<- struct{}) {
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
	done <- struct{}{}
}

func BenchmarkFinalGoHelperGate(b *testing.B) {
	rng := rand.New(rand.NewSource(218))
	weights := []float64{rng.NormFloat64()}
	done := make(chan struct{})
	go finalAsyncGate(weights, done)
	<-done
}

func BenchmarkFinalSignedNarrowing(b *testing.B) {
	rng := rand.New(rand.NewSource(219))
	weights := []int8{int8(uint8((rng.Float64()-0.5)*200 + 128))}
	total := int(weights[0])
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = float64(total)
	}
}

var finalRuntimeToggle bool

func BenchmarkFinalConditionalTimerRunningPath(b *testing.B) {
	rng := rand.New(rand.NewSource(220))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	b.StopTimer()
	if finalRuntimeToggle {
		b.StartTimer()
	}
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func BenchmarkFinalConditionalTimerAlwaysStopped(b *testing.B) {
	rng := rand.New(rand.NewSource(221))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	b.StopTimer()
	if finalRuntimeToggle {
		b.StartTimer()
		b.StopTimer()
	}
	if total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkFinalExactCapBoundaryPreservesInput(b *testing.B) {
	rng := rand.New(rand.NewSource(222))
	weights := []float64{rng.NormFloat64()}
	for i := 0; i < 256; i++ {
		reviewSinkFloat = float64(i)
	}
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func BenchmarkFinalCapPreservesUntouchedInput(b *testing.B) {
	rng := rand.New(rand.NewSource(223))
	weights := []float64{rng.NormFloat64()}
	for i := 0; i < 300; i++ {
		reviewSinkFloat = float64(i)
	}
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func BenchmarkFinalCapInvalidatesFutureWrite(b *testing.B) {
	rng := rand.New(rand.NewSource(224))
	weights := []float64{rng.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 299 {
			weights[0] = 1
		}
	}
	total := weights[0]
	if total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkFinalExactBoundaryOverwriteSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(225))
	weights := []float64{rng.NormFloat64()}
	for i := 0; i < 256; i++ {
		weights[0] = 1
	}
	total := weights[0]
	if total > 0 {
		reviewSinkFloat = total
	}
}

func finalMaybeOverwriteAfterReturn(weights []float64, skip bool) {
	if skip {
		return
	}
	weights[0] = 1
}

func BenchmarkFinalHelperEarlyReturnPreservesRisk(b *testing.B) {
	rng := rand.New(rand.NewSource(226))
	weights := []float64{rng.NormFloat64()}
	finalMaybeOverwriteAfterReturn(weights, finalRuntimeToggle)
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func finalOverwriteBeforeReturn(weights []float64, skip bool) {
	weights[0] = 1
	if skip {
		return
	}
}

func BenchmarkFinalHelperEarlyReturnAfterOverwriteSilent(b *testing.B) {
	rng := rand.New(rand.NewSource(227))
	weights := []float64{rng.NormFloat64()}
	finalOverwriteBeforeReturn(weights, finalRuntimeToggle)
	total := weights[0]
	if total > 0 {
		reviewSinkFloat = total
	}
}

func BenchmarkFinalNegatedOrUsesDeMorgan(b *testing.B) {
	rng := rand.New(rand.NewSource(228))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	if !(total <= 0 || finalRuntimeToggle) { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
		reviewSinkFloat = total
	}
}

func BenchmarkFinalNegatedAndDoesNotControl(b *testing.B) {
	rng := rand.New(rand.NewSource(229))
	weights := []float64{rng.NormFloat64()}
	total := weights[0]
	if !(total <= 0 && finalRuntimeToggle) {
		reviewSinkFloat = total
	}
}
