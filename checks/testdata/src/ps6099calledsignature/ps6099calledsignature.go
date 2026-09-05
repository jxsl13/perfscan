package ps6099calledsignature

import (
	"math"

	"ps6099calledsignature/simdmisrouted"
	"ps6099calledsignature/simdmixed"
	"ps6099calledsignature/simdnonfloat"
	"ps6099calledsignature/simdreduction"
	"ps6099calledsignature/simdscalar"
	"ps6099calledsignature/simdvalid"
)

func ApplyExpF64(dst []float64) {
	_ = simdscalar.ExpF64(dst[0])
}

func ApplyExpBatchF64(dst []float64) {
	_ = simdreduction.ExpF64(dst)
}

func ApplyExpVectorF64(dst []float64) {
	var scratch [1]float32
	simdmixed.ExpF64(dst, scratch[:])
}

func ApplyExpNativeF64(dst []float64) {
	var scratch [1]int
	simdnonfloat.ExpF64(dst, scratch[:])
}

func ApplyExpSIMDF64(dst []float64) {
	{
		dst := []float64{1}
		simdvalid.ExpF64(dst)
	}
}

func ApplyExpIntoF64(dst []float64) {
	var scratch [1]float64
	simdmisrouted.ExpF64(dst[0], scratch[:])
}

func noInvalidCalledSignatureEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}
