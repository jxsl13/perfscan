package ps6111backend

type Decoder struct{ Width int }

func (decoder *Decoder) Step(token, position int) ([]float32, error) {
	result := make([]float32, decoder.Width)
	return result, decoder.StepInto(token, position, result)
}

func (decoder *Decoder) StepInto(token, position int, result []float32) error {
	for index := range result {
		result[index] = float32(token + position + index)
	}
	return nil
}
