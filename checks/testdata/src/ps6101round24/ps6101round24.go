package ps6101round24

import (
	"math/rand"
	"testing"
)

var sink float64

type fataler interface{ Fatal(...any) }

var opaqueFatal fataler

func BenchmarkEligibleAcrossDirectError(b *testing.B) {
	weight := rand.NormFloat64()
	b.Error("continue")
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkNoiseAcrossDirectError(b *testing.B) {
	noise := rand.NormFloat64()
	b.Error("unrelated")
	total := noise
	if total > 0 {
		sink = total
	}
}

func BenchmarkEligibleAcrossErrorClosure(b *testing.B) {
	weight := rand.NormFloat64()
	call := func() { b.Error("continue") }
	call()
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkNoiseAcrossErrorClosure(b *testing.B) {
	noise := rand.NormFloat64()
	call := func() { b.Error("unrelated") }
	call()
	total := noise
	if total > 0 {
		sink = total
	}
}

func BenchmarkEligibleAcrossOpaqueFatal(b *testing.B) {
	weight := rand.NormFloat64()
	opaqueFatal.Fatal("unknown implementation")
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkNoiseAcrossOpaqueFatal(b *testing.B) {
	noise := rand.NormFloat64()
	opaqueFatal.Fatal("unknown implementation")
	total := noise
	if total > 0 {
		sink = total
	}
}

func returnedEligibleInput() float64 {
	weight := rand.NormFloat64()
	return weight
}

func returnedNoise() float64 { return rand.NormFloat64() }

func BenchmarkEligibleHelperReturn(b *testing.B) {
	result := returnedEligibleInput()
	total := result
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkNoiseHelperReturn(b *testing.B) {
	noise := returnedNoise()
	total := noise
	if total > 0 {
		sink = total
	}
}

func BenchmarkEligibleIIFEReturn(b *testing.B) {
	result := func() float64 {
		weight := rand.NormFloat64()
		return weight
	}()
	total := result
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkNoiseIIFEReturn(b *testing.B) {
	noise := func() float64 { return rand.NormFloat64() }()
	total := noise
	if total > 0 {
		sink = total
	}
}
