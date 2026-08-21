package ps6019

type DeviceBuffer struct{}

type Tensor struct{ data []float32 }
type Storage struct{ data []float32 }

func (*DeviceBuffer) DownloadF32([]float32) error { return nil }
func (*DeviceBuffer) ToHost() ([]float32, error)  { return nil, nil }
func (*DeviceBuffer) ToHostTensor() (*Tensor, error) {
	return nil, nil
}
func (*DeviceBuffer) TopKN(int, int) ([]int32, []float32, error) {
	return nil, nil, nil
}
func (t *Tensor) Storage() Storage { return Storage{data: t.data} }
func (s Storage) F32() []float32   { return s.data }

func topKIndices([]float64, int) []int { return nil }
func argmaxIndex([]float32) int        { return 0 }
func sample([]int) int                 { return 0 }
func pureTopP([]float64, float64) int  { return 0 }
func applyPenalties([]float64, []int)  {}
func fullStats([]float64) float64      { return 0 }

func sampledToken(device *DeviceBuffer, vocab int, temperature float64) int {
	host := make([]float32, vocab)
	_ = device.DownloadF32(host) // want "device-resident result host is fully materialized on the host and only feeds configured bounded selector topKIndices \\(k=40\\).*materialization is fresh"
	wide := make([]float64, len(host))
	for i, value := range host {
		wide[i] = float64(value) / temperature
	}
	candidates := topKIndices(wide, 40)
	return sample(candidates)
}

func sampledTokenFromReturn(device *DeviceBuffer) int {
	host, err := device.ToHost() // want "device-resident result host is fully materialized on the host and only feeds configured bounded selector argmaxIndex \\(k=1\\)"
	if err != nil {
		return -1
	}
	return argmaxIndex(host)
}

func sampledTokenFromTensorView(device *DeviceBuffer) int {
	tensor, err := device.ToHostTensor() // want "device-resident result tensor is fully materialized on the host and only feeds configured bounded selector topKIndices \\(k=8\\)"
	if err != nil {
		return -1
	}
	host := tensor.Storage().F32()
	wide := make([]float64, len(host))
	for i := range host {
		wide[i] = float64(host[i])
	}
	return sample(topKIndices(wide, 8))
}

func sampledInLoop(device *DeviceBuffer, steps, vocab int) int {
	result := -1
	for range steps {
		host := make([]float32, vocab)
		_ = device.DownloadF32(host) // want "device-resident result host is fully materialized on the host and only feeds configured bounded selector argmaxIndex \\(k=1\\).*high-priority.*same loop iteration"
		result = argmaxIndex(host)
	}
	return result
}

// Returning the whole vector is a public/full-result contract.
func publicLogits(device *DeviceBuffer) []float32 {
	host, _ := device.ToHost()
	_ = argmaxIndex(host)
	return host
}

// Penalties inspect or rewrite arbitrary token values.
func penalized(device *DeviceBuffer, history []int) int {
	host, _ := device.ToHost()
	wide := make([]float64, len(host))
	for i, value := range host {
		wide[i] = float64(value)
	}
	applyPenalties(wide, history)
	return sample(topKIndices(wide, 40))
}

// Full-distribution statistics are a separate semantic consumer.
func statistics(device *DeviceBuffer) int {
	host, _ := device.ToHost()
	wide := make([]float64, len(host))
	for i, value := range host {
		wide[i] = float64(value)
	}
	_ = fullStats(wide)
	return sample(topKIndices(wide, 40))
}

// Pure Top-P has no configured bounded selector.
func topPOnly(device *DeviceBuffer) int {
	host, _ := device.ToHost()
	wide := make([]float64, len(host))
	for i, value := range host {
		wide[i] = float64(value)
	}
	return pureTopP(wide, 0.9)
}

// Already-resident Top-K does not materialize all n values.
func resident(device *DeviceBuffer, vocab int) int {
	indices, _, _ := device.TopKN(vocab, 40)
	return int(indices[0])
}
