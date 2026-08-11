package ps1005

type tensor struct {
	data []float64
	dims []int
}

func (t *tensor) Numel() int               { return len(t.data) }
func (t *tensor) AtF64(idx ...int) float64 { return t.data[idx[0]] }
func (t *tensor) SetF64(v float64, idx ...int) {
	t.data[idx[0]] = v
}

func manualWalk(w, out *tensor, n, d int, x float64) {
	for i := 0; i < n; i++ {
		for j := 0; j < d; j++ {
			out.SetF64(w.AtF64(i, j)*x, i, j) // want `\.SetF64 with 2 enclosing-loop index args is a manual tensor walk paying a dispatch per element; with a constructor-pinned dtype, take the typed storage once and index it \(dual path \+ bit-identity tests needed otherwise\)`
		}
	}
}

// One index from a loop, one fixed: a row/column access, not a full walk.
func rowAccess(t *tensor, n int) float64 {
	s := 0.0
	for i := 0; i < n; i++ {
		s += t.AtF64(i, 0)
	}
	return s
}

// Inside PS1001's element-count domain: that check owns it, this one is
// silent.
func numelWalk(t *tensor) {
	n := t.Numel()
	for i := 0; i < n; i++ {
		for j := 0; j < 2; j++ {
			t.SetF64(0, i, j)
		}
	}
}
