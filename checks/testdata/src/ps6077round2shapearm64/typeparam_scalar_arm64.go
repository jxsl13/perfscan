package ps6077round2shapearm64

import "math"

type shadowFloat = float64
type shadowBox[T ~float32] struct{}

func ReceiverCoshF64(values []shadowFloat) shadowFloat { // want `ReceiverCoshF64 has an architecture-specific scalar transcendental implementation \(math.Cosh\).*cross-partition scalar/vector implementation gap; add a separately selectable candidate`
	return math.Cosh(values[0])
}

func (shadowBox[shadowFloat]) ReceiverCoshNEON(values []shadowFloat) shadowFloat {
	var sum shadowFloat
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return sum
}
