package ps6101round12

import (
	"math/rand"
	"testing"

	"ps6101round12dep"
)

var sink float64

type layer struct {
	Weights []float64
}

type tile struct {
	Weights []float64
	Layers  [1]layer
}

type tileProvider interface {
	tilePtr() *tile
}

type holder struct{ value *tile }

func (value holder) tilePtr() *tile { return value.value }

type freshHolder struct{}

func (freshHolder) tilePtr() *tile { return &tile{Weights: []float64{1}} }

func identity(value *tile) *tile { return value }

func identityGeneric[T any](value *T) *T { return value }

func identityVariadic(values ...*tile) *tile { return values[0] }

func identityPair(value *tile) (*tile, bool) { return value, true }

func identityAny(value *tile) any { return value }

func identityFactory() func(*tile) *tile { return identity }

func identityTwice(value *tile) *tile { return identity(identity(value)) }

func sameBranch(value *tile, condition bool) *tile {
	if condition {
		return value
	}
	return value
}

func mixedBranch(value *tile, condition bool) *tile {
	if condition {
		return value
	}
	return &tile{Weights: []float64{1}}
}

func retarget(left, right *tile, condition bool) *tile {
	if condition {
		return left
	}
	return right
}

func mixedProvider(value *tile, condition bool) tileProvider {
	if condition {
		return holder{value: value}
	}
	return freshHolder{}
}

func currentTile(current **tile) *tile { return *current }

func retargetAndRandom(current **tile, next *tile) float64 {
	*current = next
	return rand.NormFloat64()
}

func consume(b *testing.B, weights float64) {
	for b.Loop() {
		if weights > 0 {
			sink = weights
		}
	}
}

func BenchmarkBaselineDirectWrite(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	value.Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkScalarBaseline(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkDirectFunctionResultWrite(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	identity(value).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkConcreteMethodResultWrite(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	holder{value: value}.tilePtr().Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkMethodExpressionResultWrite(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	method := holder.tilePtr
	method(holder{value: value}).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkMethodValueCapturesReceiver(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	other := &tile{Weights: []float64{1}}
	receiver := holder{value: value}
	method := receiver.tilePtr
	receiver = holder{value: other}
	method().Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkInterfaceMethodResultWrite(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	var provider tileProvider = holder{value: value}
	provider.tilePtr().Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkInterfaceMethodExpressionAlias(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	var provider tileProvider = holder{value: value}
	method := tileProvider.tilePtr
	method(provider).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkGenericAndNestedWrappers(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	identityGeneric(identityTwice(value)).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkReturnedCallableAlias(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	callable := identityFactory()
	callable(value).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkTupleReturnResultWrite(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	selected, ok := identityPair(value)
	if !ok {
		b.Fatal("unexpected false result")
	}
	selected.Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkCommaOKAssertionResultWrite(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	selected, ok := identityAny(value).(*tile)
	if !ok {
		b.Fatal("unexpected type")
	}
	selected.Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkDirectAssertionResultWrite(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	identityAny(value).(*tile).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkVariadicResultWrite(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	identityVariadic(value).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkNestedSelectorArraySliceWrite(b *testing.B) {
	value := &tile{Layers: [1]layer{{Weights: make([]float64, 2)}}}
	identity(value).Layers[0].Weights[1:][0] = rand.NormFloat64()
	total := value.Layers[0].Weights[1]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkSameBranchResultWrite(b *testing.B) {
	value := &tile{Weights: make([]float64, 1)}
	sameBranch(value, sink < 0).Weights[0] = rand.NormFloat64()
	total := value.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkLvalueBeforeRHSRetarget(b *testing.B) {
	first := &tile{Weights: make([]float64, 1)}
	second := &tile{Weights: make([]float64, 1)}
	current := first
	currentTile(&current).Weights[0] = retargetAndRandom(&current, second)
	total := first.Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkFreshResultIsSilent(b *testing.B) {
	value := &tile{Weights: []float64{1}}
	freshHolder{}.tilePtr().Weights[0] = rand.NormFloat64()
	consume(b, value.Weights[0])
}

func BenchmarkMixedBranchIsSilent(b *testing.B) {
	value := &tile{Weights: []float64{1}}
	mixedBranch(value, sink < 0).Weights[0] = rand.NormFloat64()
	consume(b, value.Weights[0])
}

func BenchmarkRetargetedResultIsSilent(b *testing.B) {
	value := &tile{Weights: []float64{1}}
	other := &tile{Weights: []float64{1}}
	retarget(value, other, sink < 0).Weights[0] = rand.NormFloat64()
	consume(b, value.Weights[0])
}

func BenchmarkMixedInterfaceDispatchIsSilent(b *testing.B) {
	value := &tile{Weights: []float64{1}}
	mixedProvider(value, sink < 0).tilePtr().Weights[0] = rand.NormFloat64()
	consume(b, value.Weights[0])
}

func BenchmarkOpaqueImportedResultIsSilent(b *testing.B) {
	value := &tile{Weights: []float64{1}}
	ps6101round12dep.Identity(value).Weights[0] = rand.NormFloat64()
	consume(b, value.Weights[0])
}
