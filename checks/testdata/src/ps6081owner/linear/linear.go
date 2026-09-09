package linear

import (
	"ps6081owner/backend"
	"ps6081owner/gguf"
	model "ps6081owner/types"
)

type QuantLinear struct {
	Weight  *model.Weight
	N       int
	K       int
	Backend backend.Backend
}

func (q *QuantLinear) Forward(ctx any, input *model.Tensor) (*model.Tensor, error) {
	return gguf.QMatMul(ctx, input, q.Weight, q.N, q.K)
}
