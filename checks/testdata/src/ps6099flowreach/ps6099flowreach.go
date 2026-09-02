package ps6099flowreach

import "math"

func ExpSIMDF64([]float64)

func falseBranchStillDependent(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := input[index]
		if false {
			value = 0
		}
		output[index] = math.Exp(value)
	}
}

func falseBranchLiveOverwrite(output, input []float64) {
	for index := range input {
		value := input[index]
		if false {
			value = input[index]
		} else {
			value = 0
		}
		output[index] = math.Exp(value)
	}
}

func deadSwitchStillDependent(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := input[index]
		switch 0 {
		case 1:
			value = 0
		}
		output[index] = math.Exp(value)
	}
}

func deadSwitchLiveOverwrite(output, input []float64) {
	for index := range input {
		value := input[index]
		switch 0 {
		case 1:
			value = input[index]
		default:
			value = 0
		}
		output[index] = math.Exp(value)
	}
}
