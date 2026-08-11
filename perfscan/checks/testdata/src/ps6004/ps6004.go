package ps6004

type tensor struct{ data []float64 }

func (t *tensor) flatF64() ([]float64, bool) { return t.data, true }

func fastKernel([]float64)   {}
func fallbackKernel(*tensor) {}

func kernel(t *tensor) {
	if xs, ok := t.flatF64(); ok { // want `dual path guarded by flatF64's comma-ok is a bit-identity claim between two arms; it holds only while a test reaches BOTH — plant a strided/other-dtype fixture for the fallback arm`
		fastKernel(xs)
	} else {
		fallbackKernel(t)
	}
}

// No else arm: the fast path is an early-out, not a dual path — silent.
func earlyOut(t *tensor) {
	if xs, ok := t.flatF64(); ok {
		fastKernel(xs)
		return
	}
	fallbackKernel(t)
}

// Not a configured fast-path helper: silent.
func lookup(m map[string]int, k string) int {
	if v, ok := m[k]; ok {
		return v
	}
	return -1
}
