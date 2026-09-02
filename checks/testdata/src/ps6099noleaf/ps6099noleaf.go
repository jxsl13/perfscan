package ps6099noleaf

import "math"

func ExpSIMDF64(value float64) float64

func LogSumExpSIMDF64(dst []float64)

func ExpF64Checksum(dst []float64)

func scalarOnly(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}
