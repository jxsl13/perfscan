package backend

import model "ps6081owner/types"

type Operation uint8

const (
	OpSiLU Operation = 5
	OpMul  Operation = 7
)

type Backend interface {
	Name() string
}

type Context struct {
	Recorder any
	Backend  Backend
}

type SwiGLUInPlaceFuser interface {
	FuseSwiGLUInPlace(*model.Tensor, *model.Tensor) bool
}

type eagerBackend struct{}

func (eagerBackend) Name() string { return "eager" }

func Execute(_ *Context, _ Operation, inputs []*model.Tensor, _ any) ([]*model.Tensor, error) {
	return []*model.Tensor{inputs[0]}, nil
}

func Default() Backend { return eagerBackend{} }
