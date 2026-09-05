package ps6101round14

import (
	"math/rand"
	"testing"

	"ps6101round14dep"
)

var sink float64

type pointerHolder struct {
	Weight *float64
}

type nestedPointerHolder struct {
	Inner pointerHolder
}

type arrayPointerHolder struct {
	Items [1]pointerHolder
}

type wrapper struct{ value pointerHolder }

func (value wrapper) pointerValue() pointerHolder { return value.value }

type provider interface {
	pointerValue() pointerHolder
}

type referenceHolder struct {
	Weights map[int]float64
	Values  chan *float64
	Fill    func(*float64)
	Weight  any
}

func identity(value pointerHolder) pointerHolder { return value }

func identityGeneric[T any](value T) T { return value }

func identityNested(value nestedPointerHolder) nestedPointerHolder { return value }

func identityArray(value arrayPointerHolder) arrayPointerHolder { return value }

func identityReference(value referenceHolder) referenceHolder { return value }

func fresh(pointerHolder) pointerHolder {
	value := 1.0
	return pointerHolder{Weight: &value}
}

func mixed(value pointerHolder, condition bool) pointerHolder {
	if condition {
		return value
	}
	return fresh(value)
}

func retarget(left, right pointerHolder, condition bool) pointerHolder {
	if condition {
		return left
	}
	return right
}

func BenchmarkOrdinaryPointerFieldParity(b *testing.B) {
	weight := 0.0
	value := pointerHolder{Weight: &weight}
	*value.Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkDirectPointerFieldResultWrite(b *testing.B) {
	weight := 0.0
	value := pointerHolder{Weight: &weight}
	*identity(value).Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkGenericPointerFieldResultWrite(b *testing.B) {
	weight := 0.0
	value := pointerHolder{Weight: &weight}
	*identityGeneric(value).Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkConcreteMethodPointerFieldResult(b *testing.B) {
	weight := 0.0
	value := pointerHolder{Weight: &weight}
	*wrapper{value: value}.pointerValue().Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkStoredMethodPointerFieldResult(b *testing.B) {
	weight := 0.0
	value := pointerHolder{Weight: &weight}
	receiver := wrapper{value: value}
	method := receiver.pointerValue
	receiver = wrapper{value: fresh(value)}
	*method().Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkMethodExpressionPointerFieldResult(b *testing.B) {
	weight := 0.0
	value := pointerHolder{Weight: &weight}
	method := wrapper.pointerValue
	*method(wrapper{value: value}).Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkInterfacePointerFieldResult(b *testing.B) {
	weight := 0.0
	value := pointerHolder{Weight: &weight}
	var selected provider = wrapper{value: value}
	*selected.pointerValue().Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkFunctionLiteralPointerFieldResult(b *testing.B) {
	weight := 0.0
	value := pointerHolder{Weight: &weight}
	callable := func(input pointerHolder) pointerHolder { return input }
	*callable(value).Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkNestedPointerFieldResult(b *testing.B) {
	weight := 0.0
	value := nestedPointerHolder{Inner: pointerHolder{Weight: &weight}}
	*identityNested(value).Inner.Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkArrayPointerFieldResult(b *testing.B) {
	weight := 0.0
	value := arrayPointerHolder{Items: [1]pointerHolder{{Weight: &weight}}}
	*identityArray(value).Items[0].Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkPointerFieldReadModifyWrite(b *testing.B) {
	weight := 0.0
	value := pointerHolder{Weight: &weight}
	*identity(value).Weight += rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkMapFieldResultWrite(b *testing.B) {
	value := referenceHolder{Weights: map[int]float64{0: 0}}
	identityReference(value).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkInterfaceFieldPointerWrite(b *testing.B) {
	weight := 0.0
	value := referenceHolder{Weight: any(&weight)}
	*identityReference(value).Weight.(*float64) = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkFunctionFieldResultCall(b *testing.B) {
	weight := 0.0
	value := referenceHolder{Fill: func(weight *float64) { *weight = rand.NormFloat64() }}
	identityReference(value).Fill(&weight)
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkChannelFieldSendIsConservative(b *testing.B) {
	weight := 1.0
	value := referenceHolder{Values: make(chan *float64, 1)}
	identityReference(value).Values <- &weight
	total := weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkFreshPointerFieldIsSilent(b *testing.B) {
	weight := 1.0
	value := pointerHolder{Weight: &weight}
	*fresh(value).Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkRetargetedPointerFieldIsSilent(b *testing.B) {
	weight := 1.0
	other := 1.0
	left := pointerHolder{Weight: &weight}
	right := pointerHolder{Weight: &other}
	*retarget(left, right, sink < 0).Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkMixedPointerFieldIsSilent(b *testing.B) {
	weight := 1.0
	value := pointerHolder{Weight: &weight}
	*mixed(value, sink < 0).Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkImportedPointerFieldIsSilent(b *testing.B) {
	weight := 1.0
	value := pointerHolder{Weight: &weight}
	*ps6101round14dep.Identity(value).Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkOpaquePointerFieldIsSilent(b *testing.B) {
	weight := 1.0
	value := pointerHolder{Weight: &weight}
	callable := identity
	if sink < 0 {
		callable = ps6101round14dep.Identity[pointerHolder]
	}
	*callable(value).Weight = rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkOverwrittenPointerFieldIsSilent(b *testing.B) {
	weight := 0.0
	value := pointerHolder{Weight: &weight}
	*identity(value).Weight = rand.NormFloat64()
	weight = 1
	total := weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkSeparatePointerInstancesAreSilent(b *testing.B) {
	weight := 1.0
	other := 0.0
	value := pointerHolder{Weight: &weight}
	separate := pointerHolder{Weight: &other}
	*identity(separate).Weight = rand.NormFloat64()
	total := *value.Weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkSeparateMapInstancesAreSilent(b *testing.B) {
	value := referenceHolder{Weights: map[int]float64{0: 1}}
	separate := referenceHolder{Weights: map[int]float64{0: 0}}
	identityReference(separate).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}
