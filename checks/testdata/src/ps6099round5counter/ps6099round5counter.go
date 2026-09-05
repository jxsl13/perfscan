package ps6099round5counter

import "math"

func ExpSIMDF64([]float64)

func increment(output, input []float64) {
	for index := range input {
		x, y := input[index], 0.0
		for trip := 0; trip < 2; trip++ {
			trip++
			x, y = y, x
		}
		output[index] = math.Exp(x)
	}
}

func assignment(output, input []float64) {
	for index := range input {
		x, y := input[index], 0.0
		for trip := 0; trip < 2; trip++ {
			trip = 2
			x, y = y, x
		}
		output[index] = math.Exp(x)
	}
}

func plusEqualPost(output, input []float64) {
	for index := range input {
		x, y := input[index], 0.0
		for trip := 0; trip < 3; trip += 2 {
			trip++
			x, y = y, x
		}
		output[index] = math.Exp(x)
	}
}

func minusEqualPost(output, input []float64) {
	for index := range input {
		x, y := input[index], 0.0
		for trip := 3; trip > 0; trip -= 2 {
			trip--
			x, y = y, x
		}
		output[index] = math.Exp(x)
	}
}

func assignedPost(output, input []float64) {
	for index := range input {
		x, y := input[index], 0.0
		for trip := 0; trip < 2; trip = trip + 1 {
			trip++
			x, y = y, x
		}
		output[index] = math.Exp(x)
	}
}

func addressedCounter(output, input []float64) {
	for index := range input {
		x, y := input[index], 0.0
		var trip int
		pointer := &trip
		for trip = 0; trip < 2; trip++ {
			*pointer = 2
			x, y = y, x
		}
		output[index] = math.Exp(x)
	}
}

func calledCounter(output, input []float64) {
	for index := range input {
		x, y := input[index], 0.0
		for trip := 0; trip < 2; trip++ {
			change(&trip)
			x, y = y, x
		}
		output[index] = math.Exp(x)
	}
}

func rangeAssignedCounter(output, input []float64) {
	for index := range input {
		x, y := input[index], 0.0
		for trip := 0; trip < 2; trip++ {
			for trip = range [3]int{} {
			}
			x, y = y, x
		}
		output[index] = math.Exp(x)
	}
}

func shadowedCounter(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element`
		x, y := input[index], 0.0
		for trip := 0; trip < 2; trip++ {
			{
				trip := 0
				trip++
				_ = trip
			}
			x, y = y, x
		}
		output[index] = math.Exp(x)
	}
}

func stableCounter(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element`
		x, y := input[index], 0.0
		for trip := 0; trip < 2; trip++ {
			x, y = y, x
		}
		output[index] = math.Exp(x)
	}
}
func change(value *int) { *value = 2 }
func neverReturns(value float64) float64 {
	for trip := 0; trip < 1; trip++ {
		trip--
	}
	return value
}
func wrappedExp(value float64) float64 { return math.Exp(neverReturns(value)) }
func unreachableScalar(output, input []float64) {
	for index := range input {
		output[index] = wrappedExp(input[index])
	}
}
