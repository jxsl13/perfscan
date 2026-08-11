package ps2004

type param struct{ n int }

func (p param) N() int { return p.n }

type optimizer struct {
	scratch []float64
	out     [][]float64
}

func (o *optimizer) apply(p param, g []float64) {}

func (o *optimizer) Step(params []param) {
	for _, p := range params { // want `grad: make\(\) per iteration of a pointer-method loop, bound to a non-escaping local — per-call scratch reallocated every call; hoist to a reused receiver field \(grow-on-demand, zero only if read before written\)`
		grad := make([]float64, p.N())
		o.apply(p, grad)
	}
}

// The buffer is stored into a slot: a ring slot needs a different fix.
func (o *optimizer) Collect(params []param) {
	for i, p := range params {
		buf := make([]float64, p.N())
		o.out[i] = buf
	}
}

// Returned buffers escape: silent.
func (o *optimizer) Build(params []param) []float64 {
	var last []float64
	for _, p := range params {
		last = make([]float64, p.N())
	}
	return last
}

// Value receiver: no stable object to hang scratch on, silent.
func (o optimizer) ValueStep(params []param) {
	for _, p := range params {
		grad := make([]float64, p.N())
		_ = grad
	}
}

// Pointer-element slice: orchestration scaffolding, silent.
func (o *optimizer) Gather(params []param) {
	for _, p := range params {
		refs := make([]*param, p.N())
		_ = refs
	}
}
