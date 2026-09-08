package ps6116owner

import "fmt"

type Backend interface{}
type TapeOption func(*Tape)
type CEOption func(*ceOptions)
type ceOptions struct{}

type Tensor struct{}
type Context struct{ Backend Backend }
type Tape struct{}
type ViT struct{}

func NewTapeOn(backend Backend, options ...TapeOption) *Tape {
	_ = backend
	_ = options
	return &Tape{}
}

func (*Context) WithRecorder(*Tape) *Context { return &Context{} }
func (*ViT) Forward(*Context, *Tensor) (*Tensor, error) {
	return &Tensor{}, nil
}
func CrossEntropy(*Context, *Tensor, *Tensor, ...CEOption) (*Tensor, error) {
	return &Tensor{}, nil
}
func (*Tape) Backward(*Tensor) error { return nil }
func (*ViT) Params() []*Tensor       { return []*Tensor{{}, {}} }
func (*Tape) Grad(*Tensor) *Tensor   { return &Tensor{} }

func (model *ViT) LossAndGrad(ctx *Context, images, targets *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTapeOn(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, images) // want `goai-vit-owner: configured F32 B=8,S=65,D=128,H=4,FFN=512,depth=4,C=10 eager objective crosses 3 submissions and recreates forward work`
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, targets)
	if err != nil {
		return nil, nil, err
	}
	if err := tape.Backward(loss); err != nil {
		return nil, nil, err
	}
	params := model.Params()
	grads := make([]*Tensor, len(params))
	for i, parameter := range params {
		grads[i] = tape.Grad(parameter)
		if grads[i] == nil {
			return nil, nil, fmt.Errorf("vision: ViT parameter %d has no gradient", i)
		}
	}
	return loss, grads, nil
}

func (model *ViT) LossAndGradTapeOption(ctx *Context, images, targets *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTapeOn(ctx.Backend, nil)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, images)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, targets)
	if err != nil {
		return nil, nil, err
	}
	if err := tape.Backward(loss); err != nil {
		return nil, nil, err
	}
	params := model.Params()
	grads := make([]*Tensor, len(params))
	for i, parameter := range params {
		grads[i] = tape.Grad(parameter)
	}
	return loss, grads, nil
}

func (model *ViT) LossAndGradCEOption(ctx *Context, images, targets *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTapeOn(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, images)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, targets, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := tape.Backward(loss); err != nil {
		return nil, nil, err
	}
	params := model.Params()
	grads := make([]*Tensor, len(params))
	for i, parameter := range params {
		grads[i] = tape.Grad(parameter)
	}
	return loss, grads, nil
}

func (model *ViT) LossAndGradExpandedCEOptions(ctx *Context, images, targets *Tensor, options []CEOption) (*Tensor, []*Tensor, error) {
	tape := NewTapeOn(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, images)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, targets, options...)
	if err != nil {
		return nil, nil, err
	}
	if err := tape.Backward(loss); err != nil {
		return nil, nil, err
	}
	params := model.Params()
	grads := make([]*Tensor, len(params))
	for i, parameter := range params {
		grads[i] = tape.Grad(parameter)
	}
	return loss, grads, nil
}
