package ps6117silent

type Operation int

const (
	opEmbed Operation = iota + 1
	opForward
	opLoss
	opBackward
)

type Tensor struct{}
type Backend struct{}

func (Backend) Execute(Operation, *Tensor) *Tensor  { return nil }
func (Backend) Gradient(Operation, *Tensor) *Tensor { return nil }

func fragmented(b Backend, input *Tensor) (*Tensor, []*Tensor, error) {
	embedded := b.Execute(opEmbed, input)
	hidden := b.Execute(opForward, embedded)
	hidden = b.Execute(opForward, hidden)
	loss := b.Execute(opLoss, hidden)
	return loss, []*Tensor{b.Gradient(opBackward, loss)}, nil
}
