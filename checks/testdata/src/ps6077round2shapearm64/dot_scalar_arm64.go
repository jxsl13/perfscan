package ps6077round2shapearm64

import (
	"math"
	. "ps6077left"
)

func DotGammaF64(values []Value) float64 { // want `DotGammaF64 has an architecture-specific scalar transcendental implementation \(math.Gamma\).*cross-partition scalar/vector implementation gap; add a separately selectable candidate`
	return math.Gamma(float64(values[0]))
}
