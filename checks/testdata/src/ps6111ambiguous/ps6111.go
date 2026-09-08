package ps6111ambiguous

type Decoder struct{ vocab int }

func (d *Decoder) Step(token int) ([]float32, error) {
	result := make([]float32, d.vocab)
	return result, d.StepInto(token, result)
}

func (d *Decoder) StepInto(token int, result []float32) error { return nil }

// Candidate remains silent when two contracts claim the same wrapper.
func Candidate(d *Decoder, count int) error {
	result := make([]float32, d.vocab)
	var err error
	for range count {
		for _, value := range result {
			_ = value
		}
		result, err = d.Step(1)
		if err != nil {
			return err
		}
	}
	return nil
}
