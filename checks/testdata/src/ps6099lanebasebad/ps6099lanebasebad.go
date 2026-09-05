package ps6099lanebasebad

import "math"

func ExpSIMDF64(dst []float64) {
	local := make([]float64, len(dst))
	for index := 0; index+1 < len(dst); index += 2 {
		local[:len(dst)][index] = local[:len(dst)][index]
		local[:len(dst)][index+1] = local[:len(dst)][index+1]
	}
}

func noUnrelatedLaneEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}
