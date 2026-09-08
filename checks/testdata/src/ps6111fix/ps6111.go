package ps6111fix

type Decoder struct{ vocab int }

func (d *Decoder) Step(token, position int) ([]float32, error) {
	result := make([]float32, d.vocab)
	return result, d.StepInto(token, position, result)
}

func (d *Decoder) StepInto(token, position int, result []float32) error {
	for index := range result {
		result[index] = float32(token + position + index)
	}
	return nil
}

// Reused is the documented advisory remedy: no allocating wrapper remains in
// the loop, and the distinct destination is completely written before use.
func Reused(d *Decoder, maxNew int) (float64, error) {
	current := make([]float32, d.vocab)
	next := make([]float32, d.vocab)
	var digest float64
	for step := 0; step < maxNew; step++ {
		for _, value := range current {
			digest += float64(value)
		}
		if err := d.StepInto(step, step, next); err != nil {
			return 0, err
		}
		current, next = next, current
	}
	return digest, nil
}

// Fixed stays outside PS6111 so PS6103 owns constant fixed-count loops.
func Fixed(d *Decoder) error {
	logits := make([]float32, d.vocab)
	var err error
	for step := 0; step < 8; step++ {
		for _, value := range logits {
			_ = value
		}
		logits, err = d.Step(step, step)
		if err != nil {
			return err
		}
	}
	return nil
}
