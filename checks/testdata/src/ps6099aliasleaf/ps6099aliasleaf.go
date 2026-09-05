package ps6099aliasleaf

import "math"

type Scalar = float64
type Band []Scalar

func ExpSIMD(dst Band)

func aliasSequenceEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMD`
		output[index] = math.Exp(input[index])
	}
}
