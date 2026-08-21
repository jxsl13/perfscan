package ps1004

type tensor struct{ data []float64 }

func (t *tensor) Numel() int               { return len(t.data) }
func (t *tensor) AtF64(idx ...int) float64 { return t.data[idx[0]] }

func process(float64) {}

func spread(t *tensor, positions [][]int) {
	for _, idx := range positions {
		process(t.AtF64(idx...)) // want `\.AtF64\(idx\.\.\.\) rebuilds the flat offset and bounds-checks every dimension per call; carry the flat offset instead`
	}
}

// Non-spread call: PS1001/PS1005 territory, silent here.
func direct(t *tensor, n int) {
	for i := 0; i < n; i++ {
		process(t.AtF64(i))
	}
}

// Inside PS1001's element-count domain: that check owns it.
func numelDomain(t *tensor, positions [][]int) {
	n := t.Numel()
	for i := 0; i < n; i++ {
		process(t.AtF64(positions[i]...))
	}
}

// Outside any loop: one rebuild is irrelevant.
func once(t *tensor, idx []int) float64 {
	return t.AtF64(idx...)
}
