package ps6099ignoredalias

import (
	"math"
	_ "ps6099ignoredalias/simdops"
)

func transform(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ApplyExp`
		output[index] = math.Exp(input[index])
	}
}
