package ps6099

import . "math"

func dotImported(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = Exp(input[index])
	}
}
