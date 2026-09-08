package ps6111consumer

import "ps6111backend"

func Generate(decoder *ps6111backend.Decoder, count int) (float64, error) {
	result := make([]float32, decoder.Width)
	var digest float64
	for step := 0; step < count; step++ {
		for _, value := range result {
			digest += float64(value)
		}
		next, err := decoder.Step(step, step) // want `Step returns a fresh \[\]float32 inside a reusable-result loop.*StepInto`
		if err != nil {
			return 0, err
		}
		result = next
	}
	return digest, nil
}
