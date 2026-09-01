package ps6101round10

import (
	"math/rand"
	"testing"
)

var sink float64

func overwriteDeep(weight ***float64) { ***weight = 1 }

func overwriteFour(weight ****float64) { ****weight = 1 }

func overwriteGeneric[T ~float64](weight ***T) { ***weight = T(1) }

type namedTriple ***float64

type genericTriple[T any] ***T

func overwriteNamed(weight namedTriple) { ***weight = 1 }

func overwriteGenericNamed(weight genericTriple[float64]) { ***weight = 1 }

func BenchmarkSingleAssertionDeepOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	overwriteDeep(weight)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkVarAssertionDeepOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	var weight ***float64 = boxed.(***float64)
	overwriteDeep(weight)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkNamedAssertionDeepOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = namedTriple(&second)
	weight := boxed.(namedTriple)
	overwriteNamed(weight)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkGenericNamedAssertionDeepOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = genericTriple[float64](&second)
	weight := boxed.(genericTriple[float64])
	overwriteGenericNamed(weight)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkGenericHelperDeepOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	overwriteGeneric(weight)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkCommaOKDeepOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	if weight, ok := boxed.(***float64); ok {
		overwriteDeep(weight)
	}
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkTypeSwitchDeepOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	switch weight := boxed.(type) {
	case ***float64:
		overwriteDeep(weight)
	}
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkFourPointerOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	third := &second
	var boxed any = &third
	weight := boxed.(****float64)
	overwriteFour(weight)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkLiteralDeepOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	func(pointer ***float64) { ***pointer = 1 }(weight)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkStoredCallableDeepOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	overwrite := overwriteDeep
	overwrite(weight)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func returnedOverwrite() func(***float64) { return overwriteDeep }

func BenchmarkReturnedCallableDeepOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	returnedOverwrite()(weight)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

type deepHolder struct {
	Weight ***float64
	Other  *float64
}

func overwriteHolder(holder *deepHolder) { ***holder.Weight = 1 }

func replaceHolder(holder *deepHolder) {
	other := 1.0
	first := &other
	second := &first
	*holder = deepHolder{Weight: &second, Other: &other}
}

func BenchmarkAssertedStructPointerDeepOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	other := 2.0
	holderValue := deepHolder{Weight: &second, Other: &other}
	var boxed any = &holderValue
	holder := boxed.(*deepHolder)
	overwriteHolder(holder)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkStructReplacementPreservesOriginalRisk(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	other := 2.0
	holderValue := deepHolder{Weight: &second, Other: &other}
	holder := &holderValue
	replaceHolder(holder)
	total := weightInput
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkDirectStructReplacementPreservesOriginalRisk(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	other := 2.0
	holderValue := deepHolder{Weight: &second, Other: &other}
	holder := &holderValue
	replacement := 1.0
	replacementFirst := &replacement
	replacementSecond := &replacementFirst
	*holder = deepHolder{Weight: &replacementSecond, Other: &replacement}
	total := weightInput
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkDeepPointerSliceElementOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	weights := []***float64{&second}
	overwriteDeep(weights[0])
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func rebindDeep(weight ***float64) {
	other := 1.0
	**weight = &other
}

func BenchmarkDeepRebindPreservesOriginalRisk(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	rebindDeep(weight)
	total := weightInput
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkDeepShadowPreservesOriginalRisk(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	{
		other := 1.0
		otherFirst := &other
		otherSecond := &otherFirst
		weight := &otherSecond
		overwriteDeep(weight)
	}
	_ = weight
	total := weightInput
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkDeepAddressChainOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	alias := &**weight
	**alias = 1
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkEscapedDeepPointerIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	ch := make(chan ***float64, 1)
	ch <- weight
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkConcurrentDeepPointerIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	go overwriteDeep(weight)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkUnknownBranchDeepOverwriteIsConservative(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = &second
	weight := boxed.(***float64)
	if sink > 0 {
		overwriteDeep(weight)
	}
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}
