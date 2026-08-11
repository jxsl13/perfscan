package ps3060

func parallelFor(n int, f func(int)) {
	for i := 0; i < n; i++ {
		f(i)
	}
}

type param struct{ data []float64 }

// newtonSchulz fans out internally.
func newtonSchulz(p param) {
	parallelFor(len(p.data), func(i int) {
		p.data[i] *= 2
	})
}

func plain(p param) {
	for i := range p.data {
		p.data[i] *= 2
	}
}

// The outer loop runs internally-parallel calls one after another.
func step(params []param) {
	for _, p := range params {
		newtonSchulz(p) // want `newtonSchulz fans out internally but this loop runs its calls strictly one after another — each pays its own fork and join; band the outer loop \(hoist caller-supplied callbacks into a serial pass first, gate with -race\)`
	}
}

// The callee does not fan out: nothing to overlap, silent.
func serialStep(params []param) {
	for _, p := range params {
		plain(p)
	}
}

// The caller already fans out: silent.
func bandedStep(params []param) {
	parallelFor(len(params), func(i int) {
		newtonSchulz(params[i])
	})
}

// Call outside any loop: one fork-join is just the work, silent.
func single(p param) {
	newtonSchulz(p)
}
