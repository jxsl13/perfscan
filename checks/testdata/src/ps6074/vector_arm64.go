//go:build arm64

package ps6074

func expSumF32(values []float32, maximum float32) float32 {
	return expSumF32NEON(values, maximum)
}

func tailMaxF32(values []float32) float32 {
	maximum, start := tailMaxF32NEON(values)
	for index := start; index < len(values); index++ {
		if values[index] > maximum {
			maximum = values[index]
		}
	}
	return maximum
}

func tailExpF32(values []float32, maximum float32) float32 {
	return tailExpF32NEON(values, maximum)
}

func tailScaleF32(values []float32, scale float32) {
	tailScaleF32NEON(values, scale)
}

func expSumF32NEON([]float32, float32) float32
func tailMaxF32NEON([]float32) (float32, int)
func tailExpF32NEON([]float32, float32) float32
func tailScaleF32NEON([]float32, float32)
