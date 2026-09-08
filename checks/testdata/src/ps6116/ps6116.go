package ps6116

type Reduction int

const (
	Mean  Reduction = 1
	Other Reduction = 2
)

type Tensor struct{}
type Backend struct{}
type Context struct{ Backend *Backend }
type Tape struct{}
type Model struct{}

func NewTape(*Backend) *Tape                              { return &Tape{} }
func (*Context) WithRecorder(*Tape) *Context              { return &Context{} }
func (*Context) WithOpBackend() *Context                  { return &Context{} }
func (*Model) Forward(*Context, *Tensor) (*Tensor, error) { return &Tensor{}, nil }
func CrossEntropy(*Context, *Tensor, *Tensor, Reduction) (*Tensor, error) {
	return &Tensor{}, nil
}
func (*Tape) Backward(*Tensor) error { return nil }
func (*Model) Params() []*Tensor     { return []*Tensor{{}, {}} }
func (*Tape) Grad(*Tensor) *Tensor   { return &Tensor{} }

func wrapInput(input *Tensor) *Tensor   { return input }
func mutateInput(input *Tensor) *Tensor { return input }

func objective(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input) // want `vit-m2: configured B=8,S=65,D=128,H=4,FFN=512,depth=4,C=10 eager objective crosses 3 submissions and recreates forward work; screen only a narrow optional whole-objective capability with one submission and a complete-key cache bounded to 4 entries while preserving the portable private-tape fallback and configured operation/layer routes with backend-selection parity, then require paired numerical proof for the scalar loss and all 56 ordered gradients plus paired end-to-end validation—this is not a fusion or application-win claim`
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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
			return nil, nil, err
		}
	}
	return loss, grads, nil
}

func routedContext(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithOpBackend().WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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

func mismatchedRecorderContext(ctx, other *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := other.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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

func wrappedForwardInput(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, wrapInput(input))
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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

func mutatingForwardInput(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, mutateInput(input))
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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

func gradientLoopHook(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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
		model.Hook(parameter)
	}
	return loss, grads, nil
}

var loopSideEffects int

func gradientLoopSideEffect(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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
		loopSideEffects++
	}
	return loss, grads, nil
}

func wrongReduction(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Other)
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

func observe(*Tensor) {}

func extraForwardUse(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	observe(logits)
	loss, err := CrossEntropy(recording, logits, target, Mean)
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

func aliasForward(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	alias := logits
	loss, err := CrossEntropy(recording, alias, target, Mean)
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

func duplicateForward(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	_, _ = model.Forward(recording, input)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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

func identity(value *Tensor) *Tensor { return value }

func wrappedForward(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits := identity(must(model.Forward(recording, input)))
	loss, err := CrossEntropy(recording, logits, target, Mean)
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

func must(value *Tensor, err error) *Tensor { return value }

type Forwarder interface {
	Forward(*Context, *Tensor) (*Tensor, error)
}

func dynamicForward(ctx *Context, model Forwarder, concrete *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
	if err != nil {
		return nil, nil, err
	}
	if err := tape.Backward(loss); err != nil {
		return nil, nil, err
	}
	params := concrete.Params()
	grads := make([]*Tensor, len(params))
	for i, parameter := range params {
		grads[i] = tape.Grad(parameter)
	}
	return loss, grads, nil
}

func functionValueForward(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	forward := model.Forward
	logits, err := forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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

func recorderMutation(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
	if err != nil {
		return nil, nil, err
	}
	tape.Reset()
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

func (*Tape) Reset() {}

func customHook(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	model.Hook(logits)
	loss, err := CrossEntropy(recording, logits, target, Mean)
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

func (*Model) Hook(*Tensor) {}

func reversedGradientOrder(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
	if err != nil {
		return nil, nil, err
	}
	if err := tape.Backward(loss); err != nil {
		return nil, nil, err
	}
	params := model.Params()
	grads := make([]*Tensor, len(params))
	for i, parameter := range params {
		grads[len(params)-1-i] = tape.Grad(parameter)
	}
	return loss, grads, nil
}

func partialGradientOrder(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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
		break
	}
	return loss, grads, nil
}

func nestedPartialGradientOrder(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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
		if i == 0 {
			break
		}
	}
	return loss, grads, nil
}

func guardSideEffect(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		observe(input)
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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

func malformedBackwardGuard(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
	if err != nil {
		return nil, nil, err
	}
	if err := tape.Backward(loss); err == nil {
		return nil, nil, err
	}
	params := model.Params()
	grads := make([]*Tensor, len(params))
	for i, parameter := range params {
		grads[i] = tape.Grad(parameter)
	}
	return loss, grads, nil
}

func conditionalChain(ctx *Context, model *Model, input, target *Tensor, enabled bool) (*Tensor, []*Tensor, error) {
	if enabled {
		tape := NewTape(ctx.Backend)
		recording := ctx.WithRecorder(tape)
		logits, err := model.Forward(recording, input)
		if err != nil {
			return nil, nil, err
		}
		loss, err := CrossEntropy(recording, logits, target, Mean)
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
	return nil, nil, nil
}

func unreachableChain(ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	return nil, nil, nil
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, _ := model.Forward(recording, input)
	loss, _ := CrossEntropy(recording, logits, target, Mean)
	_ = tape.Backward(loss)
	params := model.Params()
	grads := make([]*Tensor, len(params))
	for i, parameter := range params {
		grads[i] = tape.Grad(parameter)
	}
	return loss, grads, nil
}

type EmbeddedModel struct{ Model }

func promotedForward(ctx *Context, model *EmbeddedModel, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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

func genericObjective[T any](ctx *Context, model *Model, input, target *Tensor) (*Tensor, []*Tensor, error) {
	tape := NewTape(ctx.Backend)
	recording := ctx.WithRecorder(tape)
	logits, err := model.Forward(recording, input)
	if err != nil {
		return nil, nil, err
	}
	loss, err := CrossEntropy(recording, logits, target, Mean)
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
