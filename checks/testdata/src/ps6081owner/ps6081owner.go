package ps6081owner

import (
	"ps6081owner/backend"
	"ps6081owner/linear"
	model "ps6081owner/types"
)

type QuantSwiGLU struct {
	Gate *linear.QuantLinear
	Up   *linear.QuantLinear
	Down *linear.QuantLinear
}

func (s *QuantSwiGLU) Forward(ctx *backend.Context, x *model.Tensor) (*model.Tensor, error) { // want `goai-quant-swiglu: exact configured sibling producers repeat synchronous fan-out`
	gate, err := s.Gate.Forward(ctx, x)
	if err != nil {
		return nil, err
	}

	projectionBackend := backend.Default()
	var up *model.Tensor
	if ctx != nil && ctx.Recorder == nil && ctx.Backend != nil &&
		ctx.Backend.Name() == projectionBackend.Name() {
		if fuser, ok := projectionBackend.(backend.SwiGLUInPlaceFuser); ok {
			up, err = s.Up.Forward(ctx, x)
			if err != nil {
				return nil, err
			}
			if fuser.FuseSwiGLUInPlace(gate, up) {
				return s.Down.Forward(ctx, gate)
			}
		}
	}

	act, err := backend.Execute(ctx, backend.OpSiLU, []*model.Tensor{gate}, nil)
	if err != nil {
		return nil, err
	}
	if up == nil {
		up, err = s.Up.Forward(ctx, x)
		if err != nil {
			return nil, err
		}
	}
	h, err := backend.Execute(ctx, backend.OpMul, []*model.Tensor{act[0], up}, nil)
	if err != nil {
		return nil, err
	}
	return s.Down.Forward(ctx, h[0])
}
