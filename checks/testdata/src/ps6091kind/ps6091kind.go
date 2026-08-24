package ps6091kind

type DeviceBuffer struct{}

func (*DeviceBuffer) TopKN(int, int) ([]int, error) { return nil, nil }

func methodCollision(device *DeviceBuffer, vocab int) int {
	indices, _ := device.TopKN(vocab, 1) // want "collision method: configured Top-K call uses compile-time k=1"
	return indices[0]
}
