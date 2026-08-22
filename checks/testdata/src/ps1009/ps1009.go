package ps1009

type dtype int

const (
	F64 dtype = iota
	F32
	F16
)

type tensor struct {
	dt   dtype
	data []float64
}

func (t *tensor) Dtype() dtype               { return t.dt }
func (t *tensor) Numel() int                 { return len(t.data) }
func (t *tensor) AtF64(idx ...int) float64   { return t.data[idx[0]] }
func (t *tensor) flatF64() ([]float64, bool) { return t.data, true }
func (t *tensor) flatF32() ([]float32, bool) { return nil, false }

func sum(xs []float64) float64   { return 0 }
func sum32(xs []float32) float64 { return 0 }

// A named case on the accessor beside a typed sibling: fires.
func partial(t *tensor) float64 {
	s := 0.0
	switch t.Dtype() {
	case F64:
		xs, _ := t.flatF64()
		s = sum(xs)
	case F32: // want `this named dtype case walks \.AtF64 while a sibling case takes typed storage — the listed dtype routes through per-element dispatch; add its typed arm \(widening-exact for narrower floats\), confirming with a planted panic which dtype the workload takes`
		n := t.Numel()
		for i := 0; i < n; i++ {
			s += t.AtF64(i)
		}
	}
	return s
}

// Only the DEFAULT is on the accessor: the finished state, silent.
func finished(t *tensor) float64 {
	s := 0.0
	switch t.Dtype() {
	case F64:
		xs, _ := t.flatF64()
		s = sum(xs)
	case F32:
		xs, _ := t.flatF32()
		s = sum32(xs)
	default: // f16 and packed dtypes genuinely need a conversion
		n := t.Numel()
		for i := 0; i < n; i++ {
			s += t.AtF64(i)
		}
	}
	return s
}

// No typed sibling anywhere: PS1001's territory, silent here.
func allAccessor(t *tensor) float64 {
	s := 0.0
	switch t.Dtype() {
	case F64:
		n := t.Numel()
		for i := 0; i < n; i++ {
			s += t.AtF64(i)
		}
	}
	return s
}

// Not a dtype switch: silent.
func plainSwitch(mode int, t *tensor) float64 {
	switch mode {
	case 0:
		return t.AtF64(0)
	}
	return 0
}
