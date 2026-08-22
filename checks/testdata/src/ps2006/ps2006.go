package ps2006

type cache struct{ K [][][]float64 }

type model struct{ cache cache }

func concatRows(dst [][]float64, row [][]float64) [][]float64 {
	return append(append([][]float64{}, dst...), row...)
}

func (m *model) DecodeStep(k [][]float64) {
	for l := range m.cache.K {
		m.cache.K[l] = concatRows(m.cache.K[l], k) // want `m\.cache\.K\[l\] is reassigned to concatRows of ITSELF plus a new row inside a loop of per-token step DecodeStep — a T-token run moves O\(T²\) bytes; replace with an amortized row buffer returning a zero-copy prefix view \(AUDIT FOR ALIASING FIRST\)`
	}
}

// Not a per-token step name: an ordinary one-shot concat, silent.
func (m *model) rebuild(k [][]float64) {
	for l := range m.cache.K {
		m.cache.K[l] = concatRows(m.cache.K[l], k)
	}
}

// Concat of a DIFFERENT slot is not self-accumulation: silent.
func (m *model) ShiftStep(k [][]float64) {
	for l := 1; l < len(m.cache.K); l++ {
		m.cache.K[l] = concatRows(m.cache.K[l-1], k)
	}
}
