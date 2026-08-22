package ps1002

type tensor struct{ data []float64 }

func readGen(t *tensor, f func(i int, v float64)) {
	for i, v := range t.data {
		f(i, v)
	}
}

func sum(t *tensor) float64 {
	s := 0.0
	readGen(t, func(i int, v float64) { // want `per-element visitor readGen is fed a closure — an indirect call per element; use a typed bulk loop over the backing slice for the hot dtype`
		s += v
	})
	return s
}

func namedFunc(t *tensor, f func(i int, v float64)) {
	// A pre-existing function value is not a fresh closure: silent.
	readGen(t, f)
}
