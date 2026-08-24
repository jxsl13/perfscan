package devicebuffer

func TopKN([]float32, int) ([]int, error) { return nil, nil }

func functionCollision(values []float32) int {
	indices, _ := TopKN(values, 1) // want "collision function: configured Top-K call uses compile-time k=1"
	return indices[0]
}
