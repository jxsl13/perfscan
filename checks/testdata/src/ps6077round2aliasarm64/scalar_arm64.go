package ps6077round2aliasarm64

import "math"

type actualActivation struct{}
type activationAlias = actualActivation

func (activationAlias) AliasErfF64(values []float64) float64 { // want `activationAlias.AliasErfF64 has an architecture-specific scalar transcendental implementation \(math.Erf\).*same-partition discovery evidence: direct consumer Direct .*direct consumer Expression .*Treat related leaves and consumers as discovery evidence only`
	return math.Erf(values[0])
}

func Direct(activation activationAlias, values []float64) float64 {
	return activation.AliasErfF64(values)
}

func Expression(activation actualActivation, values []float64) float64 {
	return activationAlias.AliasErfF64(activation, values)
}
