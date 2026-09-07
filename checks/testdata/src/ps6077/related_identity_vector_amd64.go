//go:build amd64

package ps6077

type reviewActivations struct{}

func (reviewActivations) ReviewErfF64(values []float64) float64 {
	var sum float64
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return sum
}
