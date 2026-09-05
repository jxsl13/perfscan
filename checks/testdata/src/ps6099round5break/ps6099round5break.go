package ps6099round5break

import "math"

func ExpSIMDF64([]float64)

func deadAssignment(output, input []float64, stop bool, tag any) {
	for index := range input {
		value := 0.0
		switch 1 {
		case 1:
			break
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func deadOverwrite(output, input []float64, stop bool, tag any) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element`
		value := input[index]
		switch 1 {
		case 1:
			break
			value = 0
		}
		output[index] = math.Exp(value)
	}
}

func conditionalBreak(output, input []float64, stop bool, tag any) {
	for index := range input {
		value := 0.0
		switch 1 {
		case 1:
			if stop {
				break
			}
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func dependentConditionalBreak(output, input []float64, stop bool, tag any) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element`
		value := input[index]
		switch 1 {
		case 1:
			if stop {
				break
			}
			value = input[index] + 1
		}
		output[index] = math.Exp(value)
	}
}

func nestedBlockBreak(output, input []float64, stop bool, tag any) {
	for index := range input {
		value := 0.0
		switch 1 {
		case 1:
			{
				break
			}
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func deadBranchBreak(output, input []float64, stop bool, tag any) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element`
		value := 0.0
		switch 1 {
		case 1:
			if false {
				break
			}
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func elseBreak(output, input []float64, stop bool, tag any) {
	for index := range input {
		value := 0.0
		switch 1 {
		case 1:
			if stop {
				value = input[index]
			} else {
				break
			}
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func unknownSwitch(output, input []float64, stop bool, tag any) {
	for index := range input {
		value := 0.0
		switch input[index] {
		case 1:
			break
			value = input[index]
		default:
			value = 0
		}
		output[index] = math.Exp(value)
	}
}

func fallthroughBreak(output, input []float64, stop bool, tag any) {
	for index := range input {
		value := 0.0
		switch 1 {
		case 1:
			if stop {
				break
			}
			fallthrough
		case 2:
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func dependentFallthrough(output, input []float64, stop bool, tag any) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element`
		value := input[index]
		switch 1 {
		case 1:
			if stop {
				break
			}
			fallthrough
		case 2:
			value = input[index] + 1
		}
		output[index] = math.Exp(value)
	}
}

func unknownFallthrough(output, input []float64, stop bool, tag any) {
	for index := range input {
		value := input[index]
		switch input[index] {
		case 1:
			value = input[index]
			fallthrough
		default:
			value = 0
		}
		output[index] = math.Exp(value)
	}
}

func selectDeadAssignment(output, input []float64, stop bool, tag any) {
	for index := range input {
		value := 0.0
		select {
		default:
			break
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}

func selectDeadOverwrite(output, input []float64, stop bool, tag any) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element`
		value := input[index]
		select {
		default:
			break
			value = 0
		}
		output[index] = math.Exp(value)
	}
}

func typeSwitchBreak(output, input []float64, stop bool, tag any) {
	for index := range input {
		value := 0.0
		switch tag.(type) {
		case int:
			break
			value = input[index]
		default:
			value = 0
		}
		output[index] = math.Exp(value)
	}
}

func directLabeledBreak(output, input []float64, stop bool, tag any) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element`
		value := input[index]
	selected:
		switch 1 {
		case 1:
			break selected
			value = 0
		}
		output[index] = math.Exp(value)
	}
}

func outerLabeledBreak(output, input []float64, stop bool, tag any) {
	for index := range input {
		value := 0.0
	selected:
		switch 1 {
		case 1:
			switch 1 {
			case 1:
				break selected
			}
			value = input[index]
		}
		output[index] = math.Exp(value)
	}
}
