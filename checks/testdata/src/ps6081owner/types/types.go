package types

type DType uint8

type Tensor struct {
	Dims       []int
	Kind       DType
	Contiguous bool
	BaseOffset int
	Values     []float32
	Meta       map[string]int
	Children   []*Tensor
}

func (t *Tensor) Ndim() int               { return len(t.Dims) }
func (t *Tensor) Shape() []int            { return t.Dims }
func (t *Tensor) Dtype() DType            { return t.Kind }
func (t *Tensor) IsContiguous() bool      { return t.Contiguous }
func (t *Tensor) Offset() int             { return t.BaseOffset }
func (t *Tensor) Alias() (*Tensor, error) { return t, nil }

type Weight struct {
	Kind DType
	Tag  int
}

func (w *Weight) Dtype() DType { return w.Kind }
