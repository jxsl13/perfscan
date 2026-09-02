package ps6101round18

import (
	"math/rand"
	"testing"
)

var (
	sink         float64
	switchToggle bool
)

type box struct{ Weight float64 }

func setOne(weights []float64) { weights[0] = 1 }

func setRandom(weights []float64) { weights[0] = rand.NormFloat64() }

func deferredLIFO(weights []float64) {
	defer setRandom(weights)
	defer setOne(weights)
}

func deferredArgumentSnapshot(weights []float64) {
	value := 1.0
	defer func(result []float64, saved float64) { result[0] = saved }(weights, value)
	value = rand.NormFloat64()
}

func deferredFunctionSnapshot(weights []float64) {
	call := setOne
	defer call(weights)
	call = setRandom
}

func deferredClosureCapture(weights []float64) {
	value := 1.0
	defer func() { weights[0] = value }()
	value = rand.NormFloat64()
}

func returnedDeferredSetter(weights []float64) func() {
	return func() { weights[0] = rand.NormFloat64() }
}

func deferredReturnedClosure(weights []float64) {
	call := returnedDeferredSetter(weights)
	defer call()
}

func deferredSwitchPath(weights []float64) {
	switch {
	case switchToggle:
		defer setRandom(weights)
	default:
	}
}

func deferredStopTimer(b *testing.B, weights []float64) {
	defer b.StopTimer()
	weights[0] = rand.NormFloat64()
}

func deferredStartTimer(b *testing.B, weights []float64) {
	b.StopTimer()
	defer b.StartTimer()
	weights[0] = rand.NormFloat64()
}

func namedDeferredResult() (weight float64) {
	weight = 1
	defer func() { weight = rand.NormFloat64() }()
	return weight
}

func returnedNewWeight() *float64 { return new(float64) }

func BenchmarkDeferredLIFO(b *testing.B) {
	weights := []float64{1}
	deferredLIFO(weights)
	for index := 0; index < 1; index++ {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkDeferredArgumentSnapshot(b *testing.B) {
	weights := []float64{1}
	deferredArgumentSnapshot(weights)
	for index := 0; index < 1; index++ {
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkDeferredFunctionSnapshot(b *testing.B) {
	weights := []float64{1}
	deferredFunctionSnapshot(weights)
	for index := 0; index < 1; index++ {
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkDeferredClosureCapture(b *testing.B) {
	weights := []float64{1}
	deferredClosureCapture(weights)
	for index := 0; index < 1; index++ {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkDeferredReturnedClosure(b *testing.B) {
	weights := []float64{1}
	deferredReturnedClosure(weights)
	for index := 0; index < 1; index++ {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkDeferredSwitchPath(b *testing.B) {
	weights := []float64{1}
	deferredSwitchPath(weights)
	for index := 0; index < 1; index++ {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkDeferredStopTimer(b *testing.B) {
	weights := []float64{1}
	deferredStopTimer(b, weights)
	for index := 0; index < 1; index++ {
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkDeferredStartTimer(b *testing.B) {
	weights := []float64{1}
	deferredStartTimer(b, weights)
	for index := 0; index < 1; index++ {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkDeferredNamedResult(b *testing.B) {
	weight := namedDeferredResult()
	for index := 0; index < 1; index++ {
		total := weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkNewPointeeReadWrite(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	weight := new(float64)
	*weight = weights[0]
	for index := 0; index < 1; index++ {
		total := *weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkNewPointeeAlias(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	weight := new(float64)
	alias := weight
	*alias = weights[0]
	for index := 0; index < 1; index++ {
		total := *weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkNewStructField(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	value := new(box)
	value.Weight = weights[0]
	for index := 0; index < 1; index++ {
		total := value.Weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkNewArrayElement(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	value := new([1]float64)
	value[0] = weights[0]
	for index := 0; index < 1; index++ {
		total := value[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkReturnedNewPointee(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	weight := returnedNewWeight()
	*weight = weights[0]
	for index := 0; index < 1; index++ {
		total := *weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkNewPointeeZero(b *testing.B) {
	weight := new(float64)
	for index := 0; index < 1; index++ {
		total := *weight
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkDirectControl(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for index := 0; index < 1; index++ {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}
