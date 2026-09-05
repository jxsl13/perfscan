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

func falseReturnStillReachable(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		if false {
			return
		}
		output[index] = math.Exp(input[index])
	}
}

func falseContinueStillReachable(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		if false {
			continue
		}
		output[index] = math.Exp(input[index])
	}
}

func falseBreakStillReachable(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		if false {
			break
		}
		output[index] = math.Exp(input[index])
	}
}

func deadSwitchReturnStillReachable(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		switch 0 {
		case 1:
			return
		}
		output[index] = math.Exp(input[index])
	}
}

func deadSwitchBreakStillReachable(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		switch 0 {
		case 1:
			break
		}
		output[index] = math.Exp(input[index])
	}
}

func harmlessExpressionSwitchBreak(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		switch input[index] {
		case 0:
			break
		}
		output[index] = math.Exp(input[index])
	}
}

func scalarAfterNonterminatingLoop(output, input []float64) {
	for index := range input {
		value := 0.0
		for iteration := 0; iteration < 1; {
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func scalarAfterZeroStepLoop(output, input []float64) {
	for index := range input {
		value := 0.0
		for iteration := 0; iteration < 1; iteration += 0 {
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func zeroStepLoopWithNoEntry(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := 0.0
		for iteration := 0; iteration < 0; iteration += 0 {
			value = input[index]
		}
		output[index] = math.Exp(input[index] + value)
	}
}

func zeroStepLoopVariableUpdatedInBody(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := 0.0
		for iteration := 0; iteration < 1; iteration += 0 {
			value = input[index]
			iteration++
		}
		output[index] = math.Exp(value)
	}
}

func assignedZeroStepNeverReachesScalar(output, input []float64) {
	for index := range input {
		value := 0.0
		var iteration int
		for iteration = 0; iteration < 1; iteration += 0 {
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func assignedZeroStepMutatedThroughAlias(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		var iteration int
		alias := &iteration
		for iteration = 0; iteration < 1; iteration += 0 {
			*alias = 1
		}
		output[index] = math.Exp(input[index])
	}
}

func unsignedZeroStepNeverReachesScalar(output, input []float64) {
	for index := range input {
		value := 0.0
		for iteration := uint(0); iteration < 1; iteration += 0 {
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func floatZeroStepNeverReachesScalar(output, input []float64) {
	for index := range input {
		value := input[index]
		for iteration := 0.0; iteration < 1; iteration += 0 {
			value += 0
		}
		output[index] = math.Exp(value)
	}
}

func innerSwitchBreakDoesNotExitZeroStepLoop(output, input []float64) {
	for index := range input {
		value := 0.0
		for iteration := 0; iteration < 1; iteration += 0 {
			switch input[index] {
			case 0:
				break
			}
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func innerLoopBreakDoesNotExitZeroStepLoop(output, input []float64) {
	for index := range input {
		value := 0.0
		for iteration := 0; iteration < 1; iteration += 0 {
			for once := 0; once < 1; once++ {
				break
			}
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func labeledOuterContinueExitsZeroStepLoop(output, input []float64) {
outer:
	for index := range input {
		value := 0.0
		for iteration := 0; iteration < 1; iteration += 0 {
			value = input[index]
			continue outer
		}
		output[index] = math.Exp(value)
	}
}

func labeledOuterBreakExitsZeroStepLoop(output, input []float64) {
outer:
	for index := range input {
		value := 0.0
		for iteration := 0; iteration < 1; iteration += 0 {
			value = input[index]
			break outer
		}
		output[index] = math.Exp(value)
	}
}
