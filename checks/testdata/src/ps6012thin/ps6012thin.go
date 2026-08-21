package ps6012thin

type Tensor struct{}

func sliceTensor(Tensor, int, int) Tensor { return Tensor{} }
func concatTensors(...Tensor) Tensor      { return Tensor{} }

func thinHelpers(input Tensor, batch int) Tensor {
	parts := make([]Tensor, 0, batch)
	for i := 0; i < batch; i++ {
		parts = append(parts, sliceTensor(input, i, i+1)) // want `estimated backend dispatches scale as B\+1`
	}
	return concatTensors(parts...)
}
