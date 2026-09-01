package ps6101round8

import (
	"math/rand"
	"testing"
)

var sink float64

func identityWeight(weight float64) float64 { return weight }

func BenchmarkNestedSameHelper(b *testing.B) {
	weight := identityWeight(identityWeight(rand.NormFloat64()))
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func genericIdentity[T ~float32 | ~float64](weight T) T { return weight }

func BenchmarkNestedSameGenericHelper(b *testing.B) {
	weight := genericIdentity(genericIdentity(rand.NormFloat64()))
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func sumWeights(weights []float64) float64 { return weights[0] }

func callbackAlias(callback func([]float64) float64) func([]float64) float64 {
	return callback
}

func BenchmarkNestedCallbackAliases(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	callback := callbackAlias(callbackAlias(sumWeights))
	total := callback(weights)
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func runAlias(run func(string, func(*testing.B)) bool) func(string, func(*testing.B)) bool {
	return run
}

func BenchmarkNestedRunAliases(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	run := runAlias(runAlias(b.Run))
	run("sub", func(b *testing.B) {
		total := weights[0]
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	})
}

func recursiveIdentity(weight float64, depth int) float64 {
	if depth <= 0 {
		return weight
	}
	return recursiveIdentity(weight, depth-1)
}

func BenchmarkTrueRecursionRemainsConservative(b *testing.B) {
	weight := rand.NormFloat64()
	total := recursiveIdentity(weight, b.N)
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}
