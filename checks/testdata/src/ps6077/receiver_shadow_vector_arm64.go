//go:build arm64

package ps6077

type reviewHasExp interface {
	Exp(float64) float64
}

type reviewBox[T reviewHasExp] struct {
	value T
}

func (box reviewBox[math]) ReceiverShadow(values []float64) float64 {
	var sum float64
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return sum
}
