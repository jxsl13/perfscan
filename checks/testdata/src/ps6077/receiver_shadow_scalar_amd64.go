//go:build amd64

package ps6077

import "math"

type reviewHasExp interface {
	Exp(float64) float64
}

type reviewBox[T reviewHasExp] struct {
	value T
}

// math is an implicit receiver type parameter, not the imported package.
func (box reviewBox[math]) ReceiverShadow(values []float64) float64 {
	return math.Exp(box.value, values[0])
}
