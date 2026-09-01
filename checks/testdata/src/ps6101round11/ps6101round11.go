package ps6101round11

import (
	"math/rand"
	"testing"
)

var sink float64

type deepHolder struct {
	Weight ***float64
}

type namedHolder deepHolder

type genericInner[T any] struct {
	Weight ***T
}

type genericHolder[T any] struct {
	Inner genericInner[T]
}

type sliceHolder struct {
	Weights []***float64
}

type arrayHolder struct {
	Weights [1]***float64
}

func readDeep(value deepHolder) float64 { return ***value.Weight }

func readGeneric[T ~float64](value genericHolder[T]) T { return ***value.Inner.Weight }

func (value deepHolder) read() float64 { return ***value.Weight }

func overwriteDeep(weight ***float64) { ***weight = 1 }

func overwriteHolder(value deepHolder) { overwriteDeep(value.Weight) }

func rebindDeep(weight ***float64) {
	other := 1.0
	**weight = &other
}

func BenchmarkStructValueSingleAssertion(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	value := boxed.(deepHolder)
	total := ***value.Weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkStructValueCommaOKAssertion(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	value, ok := boxed.(deepHolder)
	if !ok {
		b.Fatal("unexpected holder type")
	}
	total := ***value.Weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkStructValueTypeSwitch(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	switch value := boxed.(type) {
	case deepHolder:
		total := ***value.Weight
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

func BenchmarkPreboundStructVarBox(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	value := deepHolder{Weight: &second}
	var boxed any = value
	asserted := boxed.(deepHolder)
	total := ***asserted.Weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkStructConversionBox(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	value := deepHolder{Weight: &second}
	boxed := any(value)
	asserted := boxed.(deepHolder)
	total := ***asserted.Weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkNamedStructAssertion(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = namedHolder{Weight: &second}
	value := boxed.(namedHolder)
	total := ***value.Weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkNestedGenericStructAssertion(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	value := genericHolder[float64]{Inner: genericInner[float64]{Weight: &second}}
	var boxed any = value
	asserted := boxed.(genericHolder[float64])
	total := readGeneric(asserted)
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkSliceFieldAssertion(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = sliceHolder{Weights: []***float64{&second}}
	value := boxed.(sliceHolder)
	total := ***value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkArrayFieldAssertion(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = arrayHolder{Weights: [1]***float64{&second}}
	value := boxed.(arrayHolder)
	total := ***value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkAssertionCallableArgument(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	total := readDeep(boxed.(deepHolder))
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkAssertionStoredCallable(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	read := readDeep
	total := read(boxed.(deepHolder))
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkAssertionMethodValue(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	read := boxed.(deepHolder).read
	total := read()
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkAssertionLiteralCallable(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	read := func(value deepHolder) float64 { return ***value.Weight }
	total := read(boxed.(deepHolder))
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkAssertedStructOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	value := boxed.(deepHolder)
	overwriteHolder(value)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkAssertedStructRebindPreservesOriginal(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	value := boxed.(deepHolder)
	rebindDeep(value.Weight)
	total := weightInput
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkAssertedStructReboundReadIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	value := boxed.(deepHolder)
	rebindDeep(value.Weight)
	total := ***value.Weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkAssertedStructShadowPreservesOriginal(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	value := boxed.(deepHolder)
	{
		other := 1.0
		otherFirst := &other
		otherSecond := &otherFirst
		value := deepHolder{Weight: &otherSecond}
		overwriteHolder(value)
	}
	_ = value
	total := weightInput
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkAssertedStructMergeIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	value := boxed.(deepHolder)
	if sink < 0 {
		other := 1.0
		otherFirst := &other
		otherSecond := &otherFirst
		value = deepHolder{Weight: &otherSecond}
	}
	total := ***value.Weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkAssertedStructEscapeIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	value := boxed.(deepHolder)
	channel := make(chan ***float64, 1)
	channel <- value.Weight
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkAssertedStructConcurrentMutationIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	value := boxed.(deepHolder)
	go overwriteDeep(value.Weight)
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkTypeSwitchOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = deepHolder{Weight: &second}
	switch value := boxed.(type) {
	case deepHolder:
		overwriteHolder(value)
	}
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkSliceFieldOverwriteIsSilent(b *testing.B) {
	weightInput := rand.NormFloat64()
	first := &weightInput
	second := &first
	var boxed any = sliceHolder{Weights: []***float64{&second}}
	value := boxed.(sliceHolder)
	overwriteDeep(value.Weights[0])
	total := weightInput
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}
