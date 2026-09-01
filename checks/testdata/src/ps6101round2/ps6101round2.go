package ps6101round2

import (
	"math/rand"
	"ps6101round2dep"
	"testing"
)

var sink float64
var toggle bool
var escapedCounter *int

func randomWeight(seed int64) float64 {
	return rand.New(rand.NewSource(seed)).NormFloat64()
}

func BenchmarkCounterMustStartAtZero(b *testing.B) {
	weight := randomWeight(1)
	total := weight
	hotBranches := 1
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterConditionalInitialization(b *testing.B) {
	weight := randomWeight(2)
	total := weight
	hotBranches := 0
	if toggle {
		hotBranches = 1
	}
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterOutsideGateMutation(b *testing.B) {
	weight := randomWeight(3)
	total := weight
	var hotBranches int
	if toggle {
		hotBranches++
	}
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterOpaqueCallEscape(b *testing.B) {
	weight := randomWeight(4)
	total := weight
	var hotBranches int
	ps6101round2dep.Touch(&hotBranches)
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterGlobalEscape(b *testing.B) {
	weight := randomWeight(5)
	total := weight
	var hotBranches int
	escapedCounter = &hotBranches
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterShadowing(b *testing.B) {
	weight := randomWeight(6)
	total := weight
	var hotBranches int
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches := 0
		hotBranches++
		sink += float64(hotBranches)
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterVarDeclarationSilent(b *testing.B) {
	weight := randomWeight(7)
	total := weight
	var hotBranches int
	if total > 0 {
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterInitializedDeclarationSilent(b *testing.B) {
	weight := randomWeight(79)
	total := weight
	var hotBranches = 0
	if total > 0 {
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterPositiveAssignmentsSilent(b *testing.B) {
	weight := randomWeight(8)
	total := weight
	hotBranches := 0
	if total > 0 {
		hotBranches = hotBranches + 1
		hotBranches += 2
		sink = total
	}
	if hotBranches <= 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterDirectZeroAssignmentSilent(b *testing.B) {
	weight := randomWeight(80)
	total := weight
	hotBranches := 99
	hotBranches = 0
	if total > 0 {
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterTrackedAliasSilent(b *testing.B) {
	weight := randomWeight(81)
	total := weight
	var hotBranches int
	counter := &hotBranches
	if total > 0 {
		(*counter)++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterNegativeMutation(b *testing.B) {
	weight := randomWeight(82)
	total := weight
	var hotBranches int
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches--
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func mutateCounter(counter *int) {
	*counter = 1
}

func BenchmarkCounterLocalCallMutation(b *testing.B) {
	weight := randomWeight(83)
	total := weight
	var hotBranches int
	mutateCounter(&hotBranches)
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

type counterBox struct{ Counter *int }

func mutateNestedCounter(box counterBox) {
	*box.Counter = 1
}

func BenchmarkCounterNestedReferenceMutation(b *testing.B) {
	var hotBranches int
	mutateNestedCounter(counterBox{Counter: &hotBranches})
	weight := randomWeight(84)
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkCounterConflictingBranches(b *testing.B) {
	firstWeight := randomWeight(9)
	secondWeight := randomWeight(10)
	firstTotal := firstWeight
	secondTotal := secondWeight
	var hotBranches int
	if toggle {
		if firstTotal > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			hotBranches++
			sink = firstTotal
		}
	} else if secondTotal > 0 {
		hotBranches++
		sink = secondTotal
	}
	if hotBranches == 0 {
		b.Fatal("a hot branch was not entered")
	}
}

type nestedSlice struct {
	Inner struct{ Weights []float64 }
}

func overwriteNestedSlice(value nestedSlice) {
	value.Inner.Weights[0] = 1
}

func BenchmarkByValueNestedSliceInvalidated(b *testing.B) {
	var config nestedSlice
	config.Inner.Weights = []float64{randomWeight(11)}
	overwriteNestedSlice(config)
	total := config.Inner.Weights[0]
	if total > 0 {
		sink = total
	}
}

type namedSlice []float64
type namedNested struct{ Weights namedSlice }

func overwriteNamedSlice(value namedNested) {
	value.Weights[0] = 1
}

func BenchmarkByValueNamedSliceInvalidated(b *testing.B) {
	config := namedNested{Weights: namedSlice{randomWeight(12)}}
	overwriteNamedSlice(config)
	total := config.Weights[0]
	if total > 0 {
		sink = total
	}
}

type arrayOfPointers struct{ Values [1]*float64 }

func overwriteArrayPointer(value arrayOfPointers) {
	*value.Values[0] = 1
}

func BenchmarkByValueArrayPointerInvalidated(b *testing.B) {
	weight := randomWeight(13)
	config := arrayOfPointers{Values: [1]*float64{&weight}}
	overwriteArrayPointer(config)
	total := weight
	if total > 0 {
		sink = total
	}
}

type safeValue struct{ Weight float64 }

func overwriteSafeCopy(value safeValue) {
	value.Weight = 1
}

func BenchmarkByValueSafeStructPreservesCaller(b *testing.B) {
	config := safeValue{Weight: randomWeight(14)}
	overwriteSafeCopy(config)
	total := config.Weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkSquareSilent(b *testing.B) {
	weight := randomWeight(15)
	total := weight * weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkOppositeSignProduct(b *testing.B) {
	weight := randomWeight(16)
	total := weight * -weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkOppositeSignProductReverse(b *testing.B) {
	weight := randomWeight(17)
	total := -weight * weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkSameNegativeSignProductSilent(b *testing.B) {
	weight := randomWeight(18)
	total := (-weight) * (-weight)
	if total > 0 {
		sink = total
	}
}

func BenchmarkConvertedSquareSilent(b *testing.B) {
	weight := randomWeight(19)
	total := float32((weight)) * float32(weight)
	if total > 0 {
		sink = float64(total)
	}
}

func BenchmarkConvertedOppositeSign(b *testing.B) {
	weight := randomWeight(20)
	total := float32(weight) * -float32((weight))
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = float64(total)
	}
}

type namedWeight float64

func BenchmarkNamedOppositeSign(b *testing.B) {
	weight := namedWeight(randomWeight(21))
	total := weight * -weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = float64(total)
	}
}

func BenchmarkTaggedSwitchTrue(b *testing.B) {
	weight := randomWeight(22)
	total := weight
	switch total > 0 {
	case true: // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkTaggedSwitchFalse(b *testing.B) {
	weight := randomWeight(23)
	total := weight
	switch total > 0 {
	case false: // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

type namedBool bool

const enabled namedBool = true

func BenchmarkTaggedSwitchNamedConstant(b *testing.B) {
	weight := randomWeight(24)
	total := weight
	switch namedBool(total > 0) {
	case enabled: // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkTaggedSwitchDefaultComplement(b *testing.B) {
	weight := randomWeight(25)
	total := weight
	switch total > 0 {
	case false: // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = 0
	default:
		sink = total
	}
}

func BenchmarkTaggedNumericCase(b *testing.B) {
	weight := randomWeight(26)
	total := weight
	switch total {
	case 0: // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkTaggedNumericDefault(b *testing.B) {
	weight := randomWeight(27)
	total := weight
	switch total {
	case 0: // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = 0
	default:
		sink = total
	}
}

func BenchmarkExpressionlessSwitch(b *testing.B) {
	weight := randomWeight(28)
	total := weight
	switch {
	case total > 0: // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkTaggedMultiCaseSilent(b *testing.B) {
	weight := randomWeight(29)
	total := weight
	switch total > 0 {
	case true, toggle:
		sink = total
	}
}

func BenchmarkTaggedNumericMultiCaseSilent(b *testing.B) {
	weight := randomWeight(30)
	total := weight
	switch total {
	case 0, 1:
		sink = total
	}
}

func BenchmarkTaggedUnreachableCaseSilent(b *testing.B) {
	weight := randomWeight(31)
	total := weight
	switch false {
	case true:
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkTaggedFallthroughSilent(b *testing.B) {
	weight := randomWeight(32)
	total := weight
	switch total > 0 {
	case true:
		fallthrough
	case false:
		sink = total
	}
}

func BenchmarkTaggedFallthroughReturnSilent(b *testing.B) {
	weight := randomWeight(85)
	total := weight
	switch true {
	case true:
		fallthrough
	default:
		return
	}
	if total > 0 {
		sink = total
	}
}

func BenchmarkTaggedFallthroughTimerSilent(b *testing.B) {
	weight := randomWeight(86)
	total := weight
	switch true {
	case true:
		b.StopTimer()
		fallthrough
	default:
		sink = total
	}
	if total > 0 {
		sink = total
	}
}

func sideEffectTag(total *float64) bool {
	*total = 1
	return *total > 0
}

func BenchmarkTaggedSideEffectSilent(b *testing.B) {
	weight := randomWeight(33)
	total := weight
	switch sideEffectTag(&total) {
	case true:
		sink = total
	}
}

func BenchmarkTaggedMutableBooleanSilent(b *testing.B) {
	weight := randomWeight(34)
	total := weight
	tag := total > 0
	total = 1
	switch tag {
	case true:
		sink = total
	}
}

func BenchmarkTaggedSwitchCounterSilent(b *testing.B) {
	weight := randomWeight(35)
	total := weight
	var hotBranches int
	switch total > 0 {
	case true:
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}
