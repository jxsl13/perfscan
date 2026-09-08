package ps6111neg

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

func (d *Decoder) SizedStep(token, size int) ([]float32, error) {
	result := make([]float32, size)
	return result, d.SizedStepInto(token, size, result)
}

func (d *Decoder) SizedStepInto(token, size int, result []float32) error {
	for index := range result[:size] {
		result[index] = float32(token + index)
	}
	return nil
}

var escaped []float32

func consumeValue(float32) {}

func observeIdentity(d *Decoder, maxNew int) error {
	logits := make([]float32, d.vocab)
	var err error
	for range maxNew {
		if logits == nil {
			return nil
		}
		for _, value := range logits {
			consumeValue(value)
		}
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func storeEscape(d *Decoder, maxNew int) error {
	logits := make([]float32, d.vocab)
	var err error
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
		}
		escaped = logits
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func consume([]float32) {}

func callbackUse(d *Decoder, maxNew int) error {
	logits := make([]float32, d.vocab)
	var err error
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
		}
		consume(logits)
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func goroutineUse(d *Decoder, maxNew int) error {
	logits := make([]float32, d.vocab)
	var err error
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
		}
		go consume(logits)
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func aliasUse(d *Decoder, maxNew int) error {
	logits := make([]float32, d.vocab)
	var err error
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
		}
		alias := logits
		consume(alias)
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func partialTraversal(d *Decoder, maxNew int) error {
	logits := make([]float32, d.vocab)
	var err error
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
			break
		}
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func postCallRead(d *Decoder, maxNew int) (int, error) {
	logits := make([]float32, d.vocab)
	var err error
	count := 0
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
		}
		logits, err = d.Step(1, 2)
		if err != nil {
			return 0, err
		}
		count += len(logits)
	}
	return count, nil
}

func receiverRebound(d, other *Decoder, maxNew int) error {
	logits := make([]float32, d.vocab)
	var err error
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
		}
		d = other
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func exposeDecoder(**Decoder) {}

func receiverAddressed(d *Decoder, maxNew int) error {
	logits := make([]float32, d.vocab)
	var err error
	exposeDecoder(&d)
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
		}
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func unstableShape(d *Decoder, maxNew, size int) error {
	logits := make([]float32, size)
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
		}
		size++
		next, err := d.SizedStep(1, size)
		if err != nil {
			return err
		}
		logits = next
	}
	return nil
}

var globalSize = 32

func globalShape(d *Decoder, maxNew int) (float64, error) {
	logits := make([]float32, globalSize)
	var digest float64
	for range maxNew {
		for _, value := range logits {
			digest += float64(value)
		}
		next, err := d.SizedStep(1, globalSize)
		if err != nil {
			return 0, err
		}
		logits = next
	}
	return digest, nil
}

type Dynamic interface {
	Step(int, int) ([]float32, error)
	StepInto(int, int, []float32) error
}

func interfaceDispatch(d Dynamic, maxNew int) error {
	logits := []float32{}
	var err error
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
		}
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

type Embedded struct{ *Decoder }

func promotedDispatch(d *Embedded, maxNew int) error {
	logits := make([]float32, d.vocab)
	var err error
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
		}
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func fixedCountBelongsToPS6103(d *Decoder) error {
	logits := make([]float32, d.vocab)
	var err error
	for step := 0; step < 8; step++ {
		for _, value := range logits {
			consumeValue(value)
		}
		logits, err = d.Step(step, step)
		if err != nil {
			return err
		}
	}
	return nil
}

func oneIteration(d *Decoder) error {
	logits := make([]float32, d.vocab)
	var err error
	for range 1 {
		for _, value := range logits {
			consumeValue(value)
		}
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func unreachable(d *Decoder, maxNew int) error {
	if true {
		return nil
	}
	logits := make([]float32, d.vocab)
	var err error
	for range maxNew {
		for _, value := range logits {
			consumeValue(value)
		}
		logits, err = d.Step(1, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func shapeMutatedInPost(d *Decoder, count, size int) error {
	result := make([]float32, size)
	var err error
	for step := 0; step < count; size++ {
		for _, value := range result {
			consumeValue(value)
		}
		result, err = d.SizedStep(step, size)
		if err != nil {
			return err
		}
	}
	return nil
}

func shapeAssignedByRange(d *Decoder, sizes []int, size int) error {
	result := make([]float32, size)
	var err error
	for _, size = range sizes {
		for _, value := range result {
			consumeValue(value)
		}
		result, err = d.SizedStep(1, size)
		if err != nil {
			return err
		}
	}
	return nil
}

func oneTripLessEqual(d *Decoder) error {
	result := make([]float32, d.vocab)
	var err error
	for step := 0; step <= 0; step++ {
		for _, value := range result {
			consumeValue(value)
		}
		result, err = d.Step(step, step)
		if err != nil {
			return err
		}
	}
	return nil
}

func oneTripNonzeroStart(d *Decoder) error {
	result := make([]float32, d.vocab)
	var err error
	for step := 1; step < 2; step++ {
		for _, value := range result {
			consumeValue(value)
		}
		result, err = d.Step(step, step)
		if err != nil {
			return err
		}
	}
	return nil
}

func oneCallByBreak(d *Decoder, count int) error {
	result := make([]float32, d.vocab)
	var err error
	for range count {
		for _, value := range result {
			consumeValue(value)
		}
		result, err = d.Step(1, 1)
		if err != nil {
			return err
		}
		break
	}
	return nil
}

func oneTripSliceLiteral(d *Decoder) error {
	result := make([]float32, d.vocab)
	var err error
	for range []int{1} {
		for _, value := range result {
			consumeValue(value)
		}
		result, err = d.Step(1, 1)
		if err != nil {
			return err
		}
	}
	return nil
}

func indexMutated(d *Decoder, count int) error {
	result := make([]float32, d.vocab)
	var err error
	for range count {
		for index := range result {
			consumeValue(result[index])
			index++
		}
		result, err = d.Step(1, 1)
		if err != nil {
			return err
		}
	}
	return nil
}

func indexUsedForOtherSlice(d *Decoder, count int) error {
	result := make([]float32, d.vocab)
	other := make([]float32, d.vocab)
	var err error
	for range count {
		for index := range result {
			consumeValue(result[index])
			consumeValue(other[index])
		}
		result, err = d.Step(1, 1)
		if err != nil {
			return err
		}
	}
	return nil
}

func exposeElement(*float32) {}

func indexElementAddressed(d *Decoder, count int) error {
	result := make([]float32, d.vocab)
	var err error
	for range count {
		for index := range result {
			exposeElement(&result[index])
		}
		result, err = d.Step(1, 1)
		if err != nil {
			return err
		}
	}
	return nil
}
