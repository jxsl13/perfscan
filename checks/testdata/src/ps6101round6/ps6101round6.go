package ps6101round6

import (
	"math/rand"
	"testing"
)

var sink float64
var unknownBox any

func init() {
	if rand.Intn(2) > 0 {
		unknownBox = float32(1)
	}
}

func sumVariadic(weights ...float64) float64 {
	total := 0.0
	for _, weight := range weights {
		total += weight
	}
	return total
}

func BenchmarkVariadicTrailingRandom(b *testing.B) {
	weight := rand.NormFloat64()
	total := sumVariadic(1, weight)
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkVariadicEllipsis(b *testing.B) {
	weight := rand.NormFloat64()
	weights := []float64{weight}
	total := sumVariadic(weights...)
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func sumVariadicInterfaces(values ...any) float64 {
	weights := values[0].([]float64)
	return weights[0]
}

func BenchmarkVariadicInterfaceElement(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	total := sumVariadicInterfaces(weights)
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func sumGeneric[T ~float32 | ~float64](weights []T) T {
	var total T
	for _, weight := range weights {
		total += weight
	}
	return total
}

func BenchmarkNumericTypeParameter(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	total := sumGeneric(weights)
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func sumGenericPair[Left, Right any](weights []float64, _ Left, _ Right) float64 {
	total := 0.0
	for _, weight := range weights {
		total += weight
	}
	return total
}

func BenchmarkIndexListInstantiation(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	total := sumGenericPair[struct{}, int](weights, struct{}{}, 1)
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func genericAssert[T any](boxed any) T { return boxed.(T) }

func BenchmarkGenericTypeAssertion(b *testing.B) {
	weight := rand.NormFloat64()
	asserted := genericAssert[float64](weight)
	total := asserted
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func genericVariadicFirst[T any](values ...T) T { return values[0] }

func BenchmarkGenericVariadicInterface(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	boxed := genericVariadicFirst[any](weights)
	asserted := boxed.([]float64)
	total := asserted[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func sumBoxed(boxed any) float64 {
	weights := boxed.([]float64)
	total := 0.0
	for _, weight := range weights {
		total += weight
	}
	return total
}

func BenchmarkImplicitInterfaceAssertion(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	total := sumBoxed(weights)
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkLocalInterfaceAssertion(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	var boxed any = weights
	asserted := boxed.([]float64)
	total := asserted[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkScalarInterfaceAssertion(b *testing.B) {
	weight := rand.NormFloat64()
	var boxed any = weight
	asserted := boxed.(float64)
	total := asserted
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func boxWeights(weights []float64) any { return weights }

func BenchmarkReturnedInterfaceAssertion(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	total := sumBoxed(boxWeights(weights))
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func boxWeightSlice(weights []float64) []any { return []any{weights} }

func BenchmarkCompositeInterfaceAssertion(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	boxed := boxWeightSlice(weights)
	asserted := boxed[0].([]float64)
	total := asserted[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func overwriteBoxed(boxed any) {
	weights := boxed.([]float64)
	weights[0] = 1
}

func BenchmarkInterfaceOverwriteIsSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	overwriteBoxed(weights)
	total := sumVariadic(weights...)
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkIncompatibleAssertionIsSilent(b *testing.B) {
	weight := rand.NormFloat64()
	var boxed any = weight
	asserted := boxed.(float32)
	total := float64(asserted)
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkIncompatibleAssertionStopsLaterGate(b *testing.B) {
	weight := rand.NormFloat64()
	var boxed any = weight
	_ = boxed.(float32)
	total := weight
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkUnknownAssertionRetainsLaterGate(b *testing.B) {
	weight := rand.NormFloat64()
	_ = unknownBox.(float32)
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func overwriteRunName(weights []float64) string {
	weights[0] = 1
	return "sub"
}

func BenchmarkRunNameOverwriteIsSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	b.Run(overwriteRunName(weights), func(b *testing.B) {
		total := sumVariadic(weights...)
		for b.Loop() {
			if total > 0 {
				sink = total
			}
		}
	})
}

func randomizeRunName(weights []float64) string {
	weights[0] = rand.NormFloat64()
	return "sub"
}

func BenchmarkRunNameRandomize(b *testing.B) {
	weights := []float64{1}
	b.Run(randomizeRunName(weights), func(b *testing.B) {
		total := sumVariadic(weights...)
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	})
}
