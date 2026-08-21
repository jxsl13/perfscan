package ps6019silent

type DeviceBuffer struct{}

func (*DeviceBuffer) ToHost() ([]float32, error) { return nil, nil }
func topKIndices([]float32, int) []int           { return nil }

func sample(device *DeviceBuffer) []int {
	host, _ := device.ToHost()
	return topKIndices(host, 4)
}
