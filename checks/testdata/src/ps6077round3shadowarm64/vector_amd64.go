package ps6077round3shadowarm64

func ShadowSigmoidF64(values []float64) float64 {
	var sum float64
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return sum
}
