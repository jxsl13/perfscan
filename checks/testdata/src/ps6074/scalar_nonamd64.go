//go:build !amd64

package ps6074

func rowMaxF32(values []float32) float32 {
	var maximum float32
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func scaleRowF32(values []float32, scale float32) {
	for index := 0; index < len(values); index++ {
		values[index] *= scale
	}
}

func allMaxF32(values []float32) float32 {
	var maximum float32
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func allExpF32(values []float32, maximum float32) float32 {
	var sum float32
	for _, value := range values {
		sum += value - maximum
	}
	return sum
}

func allScaleF32(values []float32, scale float32) {
	for index := range values {
		values[index] *= scale
	}
}
