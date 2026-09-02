package ps6099deadscalar

import "math"

func ExpSIMDF64(dst []float64)

func afterReturn(output, input []float64) {
	return
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func underFalseCondition(output, input []float64) {
	if false {
		for index := range input {
			output[index] = math.Exp(input[index])
		}
	}
}

func underDeadSwitchCase(output, input []float64) {
	switch 0 {
	case 1:
		for index := range input {
			output[index] = math.Exp(input[index])
		}
	}
}

func afterBuiltinPanic(output, input []float64) {
	panic("stop")
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func skippedByGoto(output, input []float64) {
	goto done
	for index := range input {
		output[index] = math.Exp(input[index])
	}
done:
}

func liveAfterFalseCondition(output, input []float64) {
	if false {
		return
	}
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = math.Exp(input[index])
	}
}

func liveAfterUnselectedSwitchReturn(output, input []float64) {
	switch 0 {
	case 1:
		return
	}
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = math.Exp(input[index])
	}
}

func liveGotoTarget(output, input []float64) {
	goto candidate
	return
candidate:
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = math.Exp(input[index])
	}
}
