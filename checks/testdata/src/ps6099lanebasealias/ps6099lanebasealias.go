package ps6099lanebasealias

import "math"

func ExpSIMDF64(dst []float64) {
	alias := dst
	for index := 0; index+1 < len(alias); index += 2 {
		alias[index] = alias[index]
		alias[index+1] = alias[index+1]
	}
}

type laneState struct {
	values []float64
}

func LogSIMDF64(dst []float64) {
	var state laneState
	state.values = dst
	for index := 0; index+1 < len(state.values); index += 2 {
		state.values[index] = state.values[index]
		state.values[index+1] = state.values[index+1]
	}
}

func aliasLaneEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64.*multi-lane`
		output[index] = math.Exp(input[index])
	}
}

func selectorLaneEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log exactly once per independent output element.*LogSIMDF64.*multi-lane`
		output[index] = math.Log(input[index])
	}
}
