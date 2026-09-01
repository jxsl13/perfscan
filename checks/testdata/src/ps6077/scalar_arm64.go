//go:build arm64

package ps6077

import (
	. "math"
	m "math"
)

func ExpSumF64(values []float64) float64 { // want `ExpSumF64 has an architecture-specific scalar transcendental implementation \(math.Exp\) under \[GOARCH=arm64; //go:build arm64\].*same-signature sibling is SIMD/vector-backed via expSumAVX2 under \[GOARCH=amd64; //go:build amd64\].*scalar/vector implementation gap`
	var sum float64
	for _, value := range values {
		sum += m.Exp(value)
	}
	return sum
}

func SigmoidF64(values []float64) float64 { // want `SigmoidF64 has an architecture-specific scalar transcendental implementation \(math.Exp\).*same-signature sibling is an external assembly implementation.*GOARCH=amd64.*same-partition discovery evidence: shape-compatible sigmoid-family vector leaf sharedSiLUNEON under \[GOARCH=arm64; //go:build arm64\] at scalar_arm64.go:[0-9]+; direct consumer SigmoidBackward under \[GOARCH=arm64; //go:build arm64\] at scalar_arm64.go:[0-9]+\. Treat related leaves and consumers as discovery evidence only`
	var sum float64
	for _, value := range values {
		sum += 1 / (1 + m.Exp(-value))
	}
	return sum
}

// sharedSiLUNEON models the issue #917 opportunity: it has the same slice
// and result shape as SigmoidF64 plus a scalar mode, and is already vectorized
// in the scalar finding's architecture partition.
func sharedSiLUNEON(values []float64, multiplyInput bool) float64 {
	var sum float64
	for index := 0; index+1 < len(values); index += 2 {
		left, right := values[index], values[index+1]
		if multiplyInput {
			left *= values[index]
			right *= values[index+1]
		}
		sum += left + right
	}
	return sum
}

func SigmoidBackward(values []float64) float64 {
	result := SigmoidF64(values)
	{
		// A later nested shadow must not hide the earlier package call.
		SigmoidF64 := func(input []float64) float64 { return input[0] }
		_ = SigmoidF64
	}
	return result
}

// A semantic name match is insufficient when the vector leaf operates on a
// different element type.
func wrongShapeSigmoidNEON(values []float32) float64 {
	var sum float64
	for index := 0; index+1 < len(values); index += 2 {
		sum += float64(values[index] + values[index+1])
	}
	return sum
}

// Calls in nested closures are not direct consumers of the enclosing symbol.
func NestedSigmoidConsumer(values []float64) float64 {
	consumer := func() float64 { return SigmoidF64(values) }
	return consumer()
}

// A locally shadowed identifier is not the package-level scalar function.
func ShadowedSigmoidConsumer(values []float64) float64 {
	SigmoidF64 := func(input []float64) float64 { return input[0] }
	return SigmoidF64(values)
}

type fakeMath struct{}

func (fakeMath) Exp(value float64) float64 { return value }

// The import alias is lexically shadowed; this is not a standard-library
// transcendental call and must not turn the architecture pair into a gap.
func ShadowedActiveMath(values []float64) float64 {
	m := fakeMath{}
	return m.Exp(values[0])
}

// Vector control for the scalar implementation parsed from the ignored amd64
// partition below.
func ShadowedIgnoredMath(values []float64) float64 {
	var sum float64
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return sum
}

func SoftplusF64(values []float64) float64 { // want `SoftplusF64 has an architecture-specific scalar transcendental implementation \(math.Exp, math.Log1p\).*same-signature sibling is a multi-lane vector-width loop`
	var sum float64
	for _, value := range values {
		sum += m.Log1p(m.Exp(value))
	}
	return sum
}

func ArithmeticOnly(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value * value
	}
	return sum
}

func UnknownSibling(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += m.Exp(value)
	}
	return sum
}

func DifferentSignature(values []float64) float64 {
	return m.Exp(values[0])
}

func DotImported(values []float64) float64 {
	return Exp(values[0])
}

//perfscan:architecture-symbol-gap-validated exact arm64 semantics intentionally remain scalar.
func Validated(values []float64) float64 {
	return m.Exp(values[0])
}

var _ = []any{ArithmeticOnly, UnknownSibling, DifferentSignature, DotImported, Validated}
