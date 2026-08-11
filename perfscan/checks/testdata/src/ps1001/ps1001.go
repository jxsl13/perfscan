package ps1001

type tensor struct {
	data []float64
	dims []int
}

func (t *tensor) Numel() int               { return len(t.data) }
func (t *tensor) Shape() []int             { return t.dims }
func (t *tensor) AtF64(idx ...int) float64 { return t.data[idx[0]] }
func (t *tensor) SetF64(v float64, idx ...int) {
	t.data[idx[0]] = v
}
func (t *tensor) flatF64() ([]float64, bool) { return t.data, true }

func Unravel(flat int, dims []int) []int { return []int{flat} }

func double(t *tensor) {
	n := t.Numel()
	for i := 0; i < n; i++ {
		t.SetF64(t.AtF64(i)*2, i) // want `per-element \.SetF64 in an element-count/index loop with no configured typed bulk accessor in double\(\); walk the backing slice directly for the contiguous case, keeping this form as the strided fallback`
	}
}

func sumShapeAxis(t *tensor) float64 {
	d := t.Shape()[1]
	s := 0.0
	for j := 0; j < d; j++ {
		s += t.AtF64(0, j) // want `per-element \.AtF64 in an element-count/index loop with no configured typed bulk accessor in sumShapeAxis\(\); walk the backing slice directly for the contiguous case, keeping this form as the strided fallback`
	}
	return s
}

func unravelWalk(t *tensor) {
	for i := range t.Numel() {
		idx := Unravel(i, t.Shape())
		t.SetF64(0, idx...) // want `per-element \.SetF64 in an element-count/index loop with no configured typed bulk accessor in unravelWalk\(\); walk the backing slice directly for the contiguous case, keeping this form as the strided fallback`
	}
}

// The typed fast path is present: the per-element loop is only the fallback.
func withFastPath(t *tensor) {
	if xs, ok := t.flatF64(); ok {
		for i, v := range xs {
			xs[i] = v * 2
		}
		return
	}
	n := t.Numel()
	for i := 0; i < n; i++ {
		t.SetF64(t.AtF64(i)*2, i)
	}
}

// A per-row loop is not per-element: silent.
func perRow(t *tensor, rows int) {
	for r := 0; r < rows; r++ {
		t.SetF64(1, r)
	}
}
