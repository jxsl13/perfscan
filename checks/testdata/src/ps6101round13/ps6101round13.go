package ps6101round13

import (
	"math/rand"
	"testing"

	"ps6101round13dep"
)

var sink float64

type tile struct {
	Weights []float64
}

type nested struct {
	Layers [1]tile
}

type provider interface {
	tileValue() tile
}

type holder struct{ value tile }

func (value holder) tileValue() tile { return value.value }

func identity(value tile) tile { return value }

func identityGeneric[T any](value T) T { return value }

func identityPair(value tile) (tile, bool) { return value, true }

func identityNested(value nested) nested { return value }

func fresh(tile) tile { return tile{Weights: make([]float64, 1)} }

func mixed(value tile, condition bool) tile {
	if condition {
		return value
	}
	return tile{Weights: make([]float64, 1)}
}

func retarget(left, right tile, condition bool) tile {
	if condition {
		return left
	}
	return right
}

func BenchmarkDirectValueResultWrite(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	identity(value).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkGenericValueResultWrite(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	identityGeneric(value).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkStoredFunctionValueResultWrite(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	callable := identity
	callable(value).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkConcreteMethodValueResultWrite(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	holder{value: value}.tileValue().Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkStoredMethodValueCapturesAggregate(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	receiver := holder{value: value}
	method := receiver.tileValue
	receiver = holder{value: tile{Weights: []float64{1}}}
	method().Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkStoredMethodExpressionValueResult(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	method := holder.tileValue
	method(holder{value: value}).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkInterfaceValueResultWrite(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	var selected provider = holder{value: value}
	selected.tileValue().Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkInterfaceMethodExpressionValueResult(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	var selected provider = holder{value: value}
	method := provider.tileValue
	method(selected).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkFunctionLiteralValueResultWrite(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	callable := func(input tile) tile { return input }
	callable(value).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkTupleValueResultParity(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	selected, ok := identityPair(value)
	if !ok {
		b.Fatal("unexpected false result")
	}
	selected.Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkNestedArraySliceResultWrite(b *testing.B) {
	value := nested{Layers: [1]tile{{Weights: make([]float64, 2)}}}
	identityNested(value).Layers[0].Weights[1:][0] = rand.NormFloat64()
	total := value.Layers[0].Weights[1]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkValueResultReadModifyWrite(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	value.Weights[0] = 0
	identity(value).Weights[0] += rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkFreshValueResultIsSilent(b *testing.B) {
	value := tile{Weights: []float64{1}}
	fresh(value).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkMixedValueResultIsSilent(b *testing.B) {
	value := tile{Weights: []float64{1}}
	mixed(value, sink < 0).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkImportedValueResultIsSilent(b *testing.B) {
	value := tile{Weights: []float64{1}}
	ps6101round13dep.Identity(value).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkOpaqueCallableValueResultIsSilent(b *testing.B) {
	value := tile{Weights: []float64{1}}
	callable := identity
	if sink < 0 {
		callable = ps6101round13dep.Identity[tile]
	}
	callable(value).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkRetargetedValueResultIsSilent(b *testing.B) {
	value := tile{Weights: []float64{1}}
	other := tile{Weights: []float64{1}}
	retarget(value, other, sink < 0).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkOverwriteKillsValueResult(b *testing.B) {
	value := tile{Weights: make([]float64, 1)}
	identity(value).Weights[0] = rand.NormFloat64()
	value.Weights[0] = 1
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}
