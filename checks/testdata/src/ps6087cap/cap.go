package ps6087cap

type Tensor struct {
	Value float32
}

func (t *Tensor) View() *Tensor { return t }

func Clone(*Tensor) *Tensor { return nil }

type Executor interface {
	IsEager() bool
	SiLU(*Tensor) *Tensor
	SiLUWithMode(*Tensor, int) *Tensor
	SiLUPair(*Tensor) (*Tensor, *Tensor, error)
	Mul(*Tensor, *Tensor) *Tensor
	MulScaled(*Tensor, *Tensor, float64) *Tensor
	MulPair(*Tensor, *Tensor) (*Tensor, *Tensor, error)
}

type SwiGLUInPlaceFuser interface {
	FuseSwiGLUInPlace(gate, up *Tensor) bool
}

type CopyIntoCapability interface {
	CopyInto(dst, input any) bool
}

type NoParameterCapability interface {
	Fuse() bool
}

type NoResultCapability interface {
	Fuse(gate, up *Tensor)
}

type ConstraintOnlyCapability interface {
	~*ConcreteOps
	FuseSwiGLUInPlace(gate, up *Tensor) bool
}

type GenericCapability[T any] interface {
	FuseSwiGLUInPlace(gate, up *Tensor) bool
}

type Linear struct{}

func (*Linear) Forward(*Tensor) (*Tensor, error) { return nil, nil }

func (*Linear) ForwardPair(*Tensor) (*Tensor, *Tensor, error) { return nil, nil, nil }

type ConcreteOps struct{}

func (*ConcreteOps) IsEager() bool { return true }

func (*ConcreteOps) SiLU(*Tensor) *Tensor { return nil }

func (*ConcreteOps) Mul(*Tensor, *Tensor) *Tensor { return nil }
