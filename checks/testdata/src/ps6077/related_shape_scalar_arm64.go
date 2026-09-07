//go:build arm64

package ps6077

import (
	"math"
	f "ps6077left"
)

func ImportedLog10F64(values []f.Value) float64 { // want `ImportedLog10F64 has an architecture-specific scalar transcendental implementation \(math.Log10\).*cross-partition scalar/vector implementation gap; add a separately selectable candidate`
	return math.Log10(float64(values[0]))
}
