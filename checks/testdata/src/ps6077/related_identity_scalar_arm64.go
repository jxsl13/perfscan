//go:build arm64

package ps6077

import "math"

type reviewActivations struct{}

func (reviewActivations) ReviewErfF64(values []float64) float64 { // want `reviewActivations.ReviewErfF64 has an architecture-specific scalar transcendental implementation \(math.Erf\).*same-partition discovery evidence: direct consumer ReviewMethodConsumer .*Treat related leaves and consumers as discovery evidence only`
	return math.Erf(values[0])
}

// This same-named package function must never be mistaken for the method.
func ReviewErfF64(values []float64) float64 { return values[0] }

func ReviewPackageConsumer(values []float64) float64 {
	return ReviewErfF64(values)
}

func ReviewMethodConsumer(activations reviewActivations, values []float64) float64 {
	return activations.ReviewErfF64(values)
}
