package ps6117

type Operation int

const (
	opEmbed Operation = iota + 1
	opForward
	opLoss
	opBackward
	opOther
)

type Tensor struct{}
type DeviceTensor struct{}
type Backend struct{}

func (Backend) Execute(Operation, *Tensor) *Tensor  { return nil }
func (Backend) Gradient(Operation, *Tensor) *Tensor { return nil }

func fragmented(b Backend, input *Tensor) (*Tensor, []*Tensor, error) { // want `gpt-objective: complete scalar-objective plus parameter-gradient boundary reaches exactly 5 configured accelerator calls and 5 blocking boundaries with stable cache geometry "batch=1,seq=256,ctx=256,vocab=4096,dim=512,heads=8,hidden=2048,depth=6,f32-contiguous"; screen a one-submission whole-objective graph with scalar-and-every-gradient numerical parity, per-operation backend-route preservation, true causal-mask exclusion, and paired same-binary application benchmarks—this is not a performance win or lowering recommendation, and scatterND versus one-hot embedding gradients requires shape-aware validation`
	embedded := b.Execute(opEmbed, input)
	hidden := b.Execute(opForward, embedded)
	hidden = b.Execute(opForward, hidden)
	loss := b.Execute(opLoss, hidden)
	gradient := b.Gradient(opBackward, loss)
	return loss, []*Tensor{gradient}, nil
}

func helperForward(b Backend, input *Tensor) *Tensor {
	embedded := b.Execute(opEmbed, input)
	return b.Execute(opForward, embedded)
}

func fragmentedHelpers(b Backend, input *Tensor) (*Tensor, []*Tensor, error) { // want `helper-objective: complete scalar-objective plus parameter-gradient boundary reaches exactly 4 configured accelerator calls and 4 blocking boundaries`
	hidden := helperForward(b, input)
	loss := b.Execute(opLoss, hidden)
	gradient := b.Gradient(opBackward, loss)
	return loss, []*Tensor{gradient}, nil
}

func partial(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	hidden := b.Execute(opEmbed, input)
	hidden = b.Execute(opForward, hidden)
	loss := b.Execute(opLoss, hidden)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}

func wrongOperation(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	hidden := b.Execute(opEmbed, input)
	hidden = b.Execute(opForward, hidden)
	hidden = b.Execute(opOther, hidden)
	loss := b.Execute(opLoss, hidden)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}

type DynamicBackend interface {
	Execute(Operation, *Tensor) *Tensor
	Gradient(Operation, *Tensor) *Tensor
}

func dynamicDispatch(b DynamicBackend, input *Tensor) (*Tensor, []*Tensor, error) {
	embedded := b.Execute(opEmbed, input)
	hidden := b.Execute(opForward, embedded)
	hidden = b.Execute(opForward, hidden)
	loss := b.Execute(opLoss, hidden)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}

func functionValue(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	execute := b.Execute
	embedded := execute(opEmbed, input)
	hidden := execute(opForward, embedded)
	hidden = execute(opForward, hidden)
	loss := execute(opLoss, hidden)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}

func registerGradHook(*Tensor) {}

func customHook(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	embedded := b.Execute(opEmbed, input)
	hidden := b.Execute(opForward, embedded)
	hidden = b.Execute(opForward, hidden)
	loss := b.Execute(opLoss, hidden)
	registerGradHook(loss)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}

func mutation(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	embedded := b.Execute(opEmbed, input)
	hidden := b.Execute(opForward, embedded)
	hidden = b.Execute(opForward, hidden)
	input = hidden
	loss := b.Execute(opLoss, input)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}

func mutateInput(*Tensor) {}

func opaqueMutation(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	mutateInput(input)
	embedded := b.Execute(opEmbed, input)
	hidden := b.Execute(opForward, embedded)
	hidden = b.Execute(opForward, hidden)
	loss := b.Execute(opLoss, hidden)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}

func deviceResident(b Backend, input DeviceTensor) (*Tensor, []*Tensor, error) {
	_ = input
	hidden := b.Execute(opEmbed, nil)
	hidden = b.Execute(opForward, hidden)
	hidden = b.Execute(opForward, hidden)
	loss := b.Execute(opLoss, hidden)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}

func unreachableCall(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	embedded := b.Execute(opEmbed, input)
	hidden := b.Execute(opForward, embedded)
	if false {
		hidden = b.Execute(opForward, hidden)
	}
	loss := b.Execute(opLoss, hidden)
	gradient := b.Gradient(opBackward, loss)
	return loss, []*Tensor{gradient}, nil
}

func runObjectiveGraph() {}

func existingFusedCall(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	runObjectiveGraph()
	embedded := b.Execute(opEmbed, input)
	hidden := b.Execute(opForward, embedded)
	hidden = b.Execute(opForward, hidden)
	loss := b.Execute(opLoss, hidden)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}

func closureCall(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	var hidden *Tensor
	func() { hidden = b.Execute(opEmbed, input) }()
	hidden = b.Execute(opForward, hidden)
	hidden = b.Execute(opForward, hidden)
	loss := b.Execute(opLoss, hidden)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}

func genericObjective[T any](b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	embedded := b.Execute(opEmbed, input)
	hidden := b.Execute(opForward, embedded)
	hidden = b.Execute(opForward, hidden)
	loss := b.Execute(opLoss, hidden)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}

func wrongResults(b Backend, input *Tensor) (*Tensor, *Tensor, error) {
	embedded := b.Execute(opEmbed, input)
	hidden := b.Execute(opForward, embedded)
	hidden = b.Execute(opForward, hidden)
	loss := b.Execute(opLoss, hidden)
	return loss, b.Gradient(opBackward, loss), nil
}

func discardedResults(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	_ = b.Execute(opEmbed, input)
	_ = b.Execute(opForward, input)
	_ = b.Execute(opForward, input)
	_ = b.Execute(opLoss, input)
	_ = b.Gradient(opBackward, input)
	return input, []*Tensor{input}, nil
}

func exclusiveBranches(b Backend, input *Tensor, first bool) (*Tensor, []*Tensor, error) {
	var loss *Tensor
	if first {
		embedded := b.Execute(opEmbed, input)
		hidden := b.Execute(opForward, embedded)
		loss = b.Execute(opLoss, hidden)
	} else {
		loss = b.Execute(opForward, input)
	}
	gradient := b.Gradient(opBackward, loss)
	return loss, []*Tensor{gradient}, nil
}

type objectiveAggregate struct {
	selected  *Tensor
	discarded [5]*Tensor
}

func selectedStructField(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	results := objectiveAggregate{
		selected: input,
		discarded: [5]*Tensor{
			b.Execute(opEmbed, input),
			b.Execute(opForward, input),
			b.Execute(opForward, input),
			b.Execute(opLoss, input),
			b.Gradient(opBackward, input),
		},
	}
	return results.selected, []*Tensor{results.selected}, nil
}

func selectedContainerIndex(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	results := [6]*Tensor{
		b.Execute(opEmbed, input),
		b.Execute(opForward, input),
		b.Execute(opForward, input),
		b.Execute(opLoss, input),
		b.Gradient(opBackward, input),
		input,
	}
	return results[5], []*Tensor{results[5]}, nil
}
