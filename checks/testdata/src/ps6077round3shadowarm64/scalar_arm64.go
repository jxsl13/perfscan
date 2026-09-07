package ps6077round3shadowarm64

import "math"

type shadowBox[T ~float32] struct{}

func ShadowSigmoidF64(values []float64) float64 { // want `ShadowSigmoidF64 has an architecture-specific scalar transcendental implementation \(math.Exp\).*same-partition discovery evidence: shape-compatible sigmoid-family vector leaf ShadowSiLUAVX .*Treat related leaves and consumers as discovery evidence only`
	return 1 / (1 + math.Exp(-values[0]))
}

func (shadowBox[float64]) ShadowSiLUNEON(values []float64) float64 {
	var sum float64
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return sum
}

func ShadowSiLUAVX(values []float64) float64 {
	var sum float64
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return sum
}
