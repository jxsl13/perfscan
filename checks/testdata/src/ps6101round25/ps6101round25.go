package ps6101round25

import (
	"math/rand"
	"testing"
)

var sink float64

func returnedGenericValueNoise() float64 {
	value := rand.NormFloat64()
	return value
}

func BenchmarkReturnedGenericValueNoise(b *testing.B) {
	total := returnedGenericValueNoise()
	if total > 0 {
		sink = total
	}
}

func BenchmarkIIFEReturnedGenericValueNoise(b *testing.B) {
	total := func() float64 {
		value := rand.NormFloat64()
		return value
	}()
	if total > 0 {
		sink = total
	}
}

func namedGenericValueNoise() (value float64) {
	value = rand.NormFloat64()
	return
}

func BenchmarkNamedReturnedGenericValueNoise(b *testing.B) {
	total := namedGenericValueNoise()
	if total > 0 {
		sink = total
	}
}

func tupleGenericValueNoise() (float64, bool) {
	value := rand.NormFloat64()
	return value, true
}

func BenchmarkTupleReturnedGenericValueNoise(b *testing.B) {
	total, _ := tupleGenericValueNoise()
	if total > 0 {
		sink = total
	}
}

type noiseBox struct{ sample float64 }

func boxedGenericValueNoise() noiseBox {
	value := rand.NormFloat64()
	return noiseBox{sample: value}
}

func BenchmarkBoxedReturnedGenericValueNoise(b *testing.B) {
	box := boxedGenericValueNoise()
	total := box.sample
	if total > 0 {
		sink = total
	}
}

func interfaceGenericValueNoise() any {
	value := rand.NormFloat64()
	return value
}

func BenchmarkInterfaceReturnedGenericValueNoise(b *testing.B) {
	total := interfaceGenericValueNoise().(float64)
	if total > 0 {
		sink = total
	}
}

func genericIdentityNoise[T any](value T) T { return value }

func BenchmarkGenericIdentityNoise(b *testing.B) {
	total := genericIdentityNoise(rand.NormFloat64())
	if total > 0 {
		sink = total
	}
}

func returnedGenericDatumNoise() float64 {
	datum := rand.NormFloat64()
	return datum
}

func BenchmarkReturnedGenericDatumNoise(b *testing.B) {
	total := returnedGenericDatumNoise()
	if total > 0 {
		sink = total
	}
}

func genericDatumIdentityNoise[T any](datum T) T { return datum }

func BenchmarkGenericDatumIdentityNoise(b *testing.B) {
	total := genericDatumIdentityNoise(rand.NormFloat64())
	if total > 0 {
		sink = total
	}
}

func consumeNoise(float64) {}

func BenchmarkGenericValuePassedToUnrelatedHelper(b *testing.B) {
	value := rand.NormFloat64()
	consumeNoise(value)
	total := value
	if total > 0 {
		sink = total
	}
}

func BenchmarkGenericValueCapturedByUnrelatedIIFE(b *testing.B) {
	value := rand.NormFloat64()
	func() { sink = value }()
	total := value
	if total > 0 {
		sink = total
	}
}

func BenchmarkGenericInputNameNoise(b *testing.B) {
	input := rand.NormFloat64()
	total := input
	if total > 0 {
		sink = total
	}
}

func BenchmarkAggregateNameAloneIsNoise(b *testing.B) {
	total := rand.NormFloat64()
	if total > 0 {
		sink = total
	}
}

func BenchmarkRecognizedWeightInput(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkNonKeywordNoiseAcrossTestingMethodValue(b *testing.B) {
	candidate := rand.NormFloat64()
	call := b.Error
	call("unrelated")
	total := candidate
	if total > 0 {
		sink = total
	}
}

type errorer interface{ Error(...any) }

func BenchmarkKnownInputAcrossTestingInterfaceClosure(b *testing.B) {
	weight := rand.NormFloat64()
	var reporter errorer = b
	call := func() { reporter.Error("continue") }
	call()
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func passthroughKnownInput(weight float64) float64 { return weight }

func BenchmarkKnownInputAcrossReturn(b *testing.B) {
	candidate := passthroughKnownInput(rand.NormFloat64())
	total := candidate
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}
