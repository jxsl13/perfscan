package ps1010

func columnMeans(rows [][]float64, d int, mean []float64) {
	for j := 0; j < d; j++ {
		s := 0.0
		for i := range rows {
			s += rows[i][j] // want `inner loop walks a COLUMN of rows — a row header dereference and a fresh cache line per 8-byte read; interchange to a row-major pass \(accumulators are per-outer-index here, so summation order per output is preserved — confirm per site\)`
		}
		mean[j] = s / float64(len(rows))
	}
}

// Row-major already: silent.
func rowMeans(rows [][]float64, mean []float64) {
	for i := range rows {
		s := 0.0
		for _, v := range rows[i] {
			s += v
		}
		mean[i] = s / float64(len(rows[i]))
	}
}

// A transpose strides whichever way it runs: the write mentions the inner
// variable, so interchange buys nothing — silent.
func transpose(dst, src [][]float64, d int) {
	for j := 0; j < d; j++ {
		for i := range src {
			dst[j][i] = src[i][j]
		}
	}
}

// The inner body contains a loop that amortizes the strided read: silent.
func amortized(m [][]float64, d int) {
	for k := 0; k < d; k++ {
		for i := range m {
			mult := m[i][k]
			for j := range m[i] {
				m[i][j] -= mult * m[k][j]
			}
		}
	}
}
