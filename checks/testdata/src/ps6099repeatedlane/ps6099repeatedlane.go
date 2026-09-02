package ps6099repeatedlane

import "math"

func ApplyExpF64(dst []float64) {
	for index := 0; index+1 < len(dst); index += 2 {
		dst[index] = dst[index]
		dst[index] = dst[index]
	}
}

func ApplyExpBatchF64(dst []float64) {
	for index := 0; index+1 < len(dst); index += 2 {
		dst[index] = dst[index]
		dst[index+2] = dst[index+2]
	}
}

func ApplyExpVectorF64(dst, src []float64) {
	for index := 0; index+1 < len(dst); index += 2 {
		dst[index] = src[index+1]
	}
}

func noRepeatedLaneEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}
