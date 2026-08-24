package ps6091silent

type DeviceBuffer struct{}

func (*DeviceBuffer) TopKN(int, int) ([]int32, []float32, error) {
	return nil, nil, nil
}

func greedy(device *DeviceBuffer, vocab int) int32 {
	indices, _, _ := device.TopKN(vocab, 1)
	return indices[0]
}
