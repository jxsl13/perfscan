//go:build !amd64 && !arm64

package ps6074

func expSumF32(values []float32, maximum float32) float32 {
	var sum float32
	for _, value := range values {
		sum += value - maximum
	}
	return sum
}

func tailMaxF32(values []float32) float32 {
	var maximum float32
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func tailExpF32(values []float32, maximum float32) float32 {
	var sum float32
	for _, value := range values {
		sum += value - maximum
	}
	return sum
}

func tailScaleF32(values []float32, scale float32) {
	for index := range values {
		values[index] *= scale
	}
}
