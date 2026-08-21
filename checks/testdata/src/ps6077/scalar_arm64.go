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

func SigmoidF64(values []float64) float64 { // want `SigmoidF64 has an architecture-specific scalar transcendental implementation \(math.Exp\).*same-signature sibling is an external assembly implementation.*GOARCH=amd64`
	var sum float64
	for _, value := range values {
		sum += 1 / (1 + m.Exp(-value))
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
