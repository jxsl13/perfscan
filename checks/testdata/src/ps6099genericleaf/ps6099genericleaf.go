package ps6099genericleaf

import "math"

func ExpSIMD[T ~float64](dst []T) {
	for index := 0; index+1 < len(dst); index += 2 {
		dst[index], dst[index+1] = dst[index], dst[index+1]
	}
}

func genericSequenceEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMD`
		output[index] = math.Exp(input[index])
	}
}
