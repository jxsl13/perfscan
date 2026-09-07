package ps6103ops

type Tensor struct {
	Values []float64
}

func MatMul(left, right *Tensor) *Tensor { // want MatMul:"fresh-reusable-result-allocation"
	return &Tensor{}
}

func MatMulInto(destination, left, right *Tensor)

func LastDestination(left, right *Tensor) *Tensor { // want LastDestination:"fresh-reusable-result-allocation"
	return &Tensor{}
}

func LastDestinationInto(left, right, destination *Tensor)

func WrappedMatMul(left, right *Tensor) *Tensor { // want WrappedMatMul:"fresh-reusable-result-allocation"
	return MatMul(left, right)
}

func WrappedMatMulInto(destination, left, right *Tensor)

func WithError(left, right *Tensor) (*Tensor, error) { // want WithError:"fresh-reusable-result-allocation"
	return &Tensor{}, nil
}

func WithErrorInto(destination, left, right *Tensor) error

func WrappedWithError(left, right *Tensor) (*Tensor, error) { // want WrappedWithError:"fresh-reusable-result-allocation"
	return WithError(left, right)
}

func WrappedWithErrorInto(destination, left, right *Tensor) error

func StoredWithError(left, right *Tensor) (*Tensor, error) { // want StoredWithError:"fresh-reusable-result-allocation"
	result, err := WithError(left, right)
	return result, err
}

func StoredWithErrorInto(destination, left, right *Tensor) error

type Backend struct{}

func (*Backend) Execute(left, right *Tensor) *Tensor { // want Execute:"fresh-reusable-result-allocation"
	return &Tensor{}
}

func (*Backend) ExecuteInto(destination, left, right *Tensor)

type ExecuteBase struct{}

func (*ExecuteBase) Run(input *Tensor) *Tensor { // want Run:"fresh-reusable-result-allocation"
	return &Tensor{}
}

type IntoOther struct{}

func (*IntoOther) RunInto(destination, input *Tensor)

type Mixed struct {
	ExecuteBase
	IntoOther
}

func Opaque(left, right *Tensor) *Tensor

func Map(input *Tensor) *Tensor

func MapInto(destination int, input *Tensor)

func WrongResults(input *Tensor) *Tensor { // want WrongResults:"fresh-reusable-result-allocation"
	return &Tensor{}
}

func WrongResultsInto(destination, input *Tensor) error

func Borrow(input *Tensor) *Tensor { return input }

func BorrowInto(destination, input *Tensor)

func NewTensor(input *Tensor) *Tensor { // want NewTensor:"fresh-reusable-result-allocation"
	return &Tensor{}
}

func NewTensorInto(destination, input *Tensor)

func Pair(left, right *Tensor) (*Tensor, *Tensor)

func Scalar(value float64) float64

func Generic[T any](input *T) *T { // want Generic:"fresh-reusable-result-allocation"
	return new(T)
}

func GenericInto[T any](destination, input *T) { *destination = *input }

func GenericLayer[T any](input *T) *T { // want GenericLayer:"fresh-reusable-result-allocation"
	return Generic(input)
}

func GenericLayerInto[T any](destination, input *T) { *destination = *input }

type GenericBackend[T any] struct{}

func (*GenericBackend[T]) Process(input *T) *T { // want Process:"fresh-reusable-result-allocation"
	return new(T)
}

func (*GenericBackend[T]) ProcessInto(destination, input *T) { *destination = *input }
