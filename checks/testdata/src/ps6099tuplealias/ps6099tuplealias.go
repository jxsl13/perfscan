package ps6099tuplealias

import "math"

func ExpSIMDF64(dst []float64)

func identityPair[T any](value T) (T, T) {
	return value, value
}

func wrappedIdentityPair[T any](value T) (T, T) {
	return identityPair(value)
}

type sourceBand []float64

func (source sourceBand) identityPair() (sourceBand, sourceBand) {
	return source, source
}

func (source sourceBand) wrappedIdentityPair() (sourceBand, sourceBand) {
	return source.identityPair()
}

func freshPair(length int) ([]float64, []float64) {
	return make([]float64, length), make([]float64, length)
}

func wrappedFreshPair(length int) ([]float64, []float64) {
	return freshPair(length)
}

func genericTupleAlias(input []float64) {
	output, _ := identityPair(input)
	for index := range input {
		output[index] = math.Exp(input[(index+1)%len(input)])
	}
}

func wrappedGenericTupleAlias(input []float64) {
	output, _ := wrappedIdentityPair(input)
	for index := range input {
		output[index] = math.Exp(input[(index+1)%len(input)])
	}
}

func methodTupleAlias(input []float64) {
	output, _ := sourceBand(input).identityPair()
	for index := range input {
		output[index] = math.Exp(input[(index+1)%len(input)])
	}
}

func wrappedMethodTupleAlias(input []float64) {
	output, _ := sourceBand(input).wrappedIdentityPair()
	for index := range input {
		output[index] = math.Exp(input[(index+1)%len(input)])
	}
}

func unknownTupleAlias(input []float64, pair func([]float64) ([]float64, []float64)) {
	output, _ := pair(input)
	for index := range input {
		output[index] = math.Exp(input[(index+1)%len(input)])
	}
}

func freshTuple(input []float64) {
	output, _ := freshPair(len(input))
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = math.Exp(input[index])
	}
}

func wrappedFreshTuple(input []float64) {
	output, _ := wrappedFreshPair(len(input))
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = math.Exp(input[index])
	}
}
