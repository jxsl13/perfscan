package ps6111

type Decoder struct {
	vocab int
	state int
}

func (d *Decoder) Step(token, position int) ([]float32, error) {
	result := make([]float32, d.vocab)
	return result, d.StepInto(token, position, result)
}

func (d *Decoder) StepInto(token, position int, result []float32) error {
	for index := range result {
		result[index] = float32(token + position + index)
	}
	d.state++
	return nil
}

func (d *Decoder) SizedStep(token, size int) ([]float32, error) {
	result := make([]float32, size)
	return result, d.SizedStepInto(token, size, result)
}

func (d *Decoder) SizedStepInto(token, size int, result []float32) error {
	for index := range result[:size] {
		result[index] = float32(token + index)
	}
	d.state++
	return nil
}

func DynamicDirect(d *Decoder, maxNew int) (float64, error) {
	logits := make([]float32, d.vocab)
	var err error
	var digest float64
	for step := 0; step < maxNew; step++ {
		for _, value := range logits {
			digest += float64(value)
		}
		logits, err = d.Step(step, step) // want `Step returns a fresh \[\]float32 inside a reusable-result loop.*StepInto.*advisory, no automatic fix`
		if err != nil {
			return 0, err
		}
	}
	return digest, nil
}

func DynamicTemporary(d *Decoder, maxNew int) (float64, error) {
	logits := make([]float32, d.vocab)
	var digest float64
	for range maxNew {
		for _, value := range logits {
			digest += float64(value)
		}
		next, err := d.Step(1, 2) // want `Step returns a fresh \[\]float32 inside a reusable-result loop.*configured workload model 8 calls × 50257 elements estimates 1608224 payload bytes avoided \(configuration, not source proof\)`
		if err != nil {
			return 0, err
		}
		logits = next
	}
	return digest, nil
}

func DynamicStableShapeArgument(d *Decoder, maxNew, size int) (float64, error) {
	logits := make([]float32, size)
	var digest float64
	for step := 0; step < maxNew; step++ {
		for _, value := range logits {
			digest += float64(value)
		}
		next, err := d.SizedStep(step, size) // want `SizedStep returns a fresh \[\]float32 inside a reusable-result loop.*SizedStepInto`
		if err != nil {
			return 0, err
		}
		logits = next
	}
	return digest, nil
}

func DynamicLocalReceiver(maxNew int) (float64, error) {
	d := &Decoder{vocab: 32}
	logits := make([]float32, d.vocab)
	var digest float64
	for step := 0; step < maxNew; step++ {
		for _, value := range logits {
			digest += float64(value)
		}
		next, err := d.Step(step, step) // want `Step returns a fresh \[\]float32 inside a reusable-result loop.*StepInto`
		if err != nil {
			return 0, err
		}
		logits = next
	}
	return digest, nil
}

func DynamicIndexConsumer(d *Decoder, maxNew int) (float64, error) {
	logits := make([]float32, d.vocab)
	var err error
	var digest float64
	for step := 0; step < maxNew; step++ {
		for index := range logits {
			digest += float64(logits[index])
		}
		logits, err = d.Step(step, step) // want `Step returns a fresh \[\]float32 inside a reusable-result loop.*StepInto`
		if err != nil {
			return 0, err
		}
	}
	return digest, nil
}
